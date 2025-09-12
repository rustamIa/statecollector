package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"log/slog"
	"time"

	"golang.org/x/sync/errgroup" //менеджер горутин удобен, когда нужно запустить несколько задач параллельно, дождаться их завершения и аккуратно обойтись с ошибками и отменой по контексту.

	"main/billingstat"
	"main/config"

	"main/incidentdata"
	"main/internal/model"

	email "main/emaildata"
	mms "main/mmsdata"
	sms "main/smsdata"
	"main/support"
	voice "main/voicedata"
)

// LogCfg описывает параметры логирования, которые удобнее всего задавать флагами/ENV.
type LogCfg struct {
	Format string // text | json
	Level  string // debug | info | warn | error
}

// readLogCfg парсит флаги командной строки и возвращает конфиг логгера.
// запукать в терминале bash
//
//	$ go run . -log.format=json -log.level=debug    go run . -log.format=text -log.level=debug
func readLogCfg() LogCfg {
	var cfg LogCfg
	flag.StringVar(&cfg.Format, "log.format", "text", "log output format: text|json")
	flag.StringVar(&cfg.Level, "log.level", "info", "log level: debug|info|warn|error")
	flag.Parse()
	return cfg
}

// -----------------------------------------
func init() {

}

func main() {
	//Конфига флагов запуска сервиса
	appFlags := readLogCfg()

	// 1. загружаем конфиг
	cfgApp, err := config.Load("config.cfg")
	if err != nil {
		// логгера ещё нет, поэтому просто stderr + выход
		_, _ = os.Stderr.WriteString("state_Collector config load error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// 2. настраиваем логирование
	logger := setupLogger(appFlags)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Дополняем контекст логгером, чтобы его можно было
	// извлекать в глубине (slog.FromContext(ctx)).
	//ctx = slog.NewContext(ctx, logger) - хоть и с версии 1.22 такое должно работать, у меня на 1.24 нет

	//logger.Debug("logging started")
	logger.Info("state_Collector starting", slog.String("Version", "1.06"))

	//TODO: добавьте ему обработку адреса “/” на функцию handleConnection
	/*router := mux.NewRouter()

	// router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// 	// an example API handler
	// 	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	// })
	router.HandleFunc("/", handleConnection)

	srv := &http.Server{
		Handler: router,
		Addr:    "127.0.0.1:8282",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	res.PrepaireResStub()
	log.Fatal(srv.ListenAndServe())*/

	// Главная работа сервиса.
	if err := run(ctx, logger, cfgApp); err != nil {
		logger.Error("collector failed", slog.Any("err", err))
	}

	logger.Info("state_Collector stopped")
}

func handleConnection(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "ok")
}

// -----------------------------------------
// тип функции, возвращающей слайс данных
type sliceFetchFn[T any] func(ctx context.Context) ([]T, error) //это компактный способ описать «контракт» для функций вида “дай мне слайс T по контексту, либо ошибку”, который можно переиспользовать для разных доменных типов.

// общий шаблон: запускает задачу в errgroup, логирует результат и НЕ отменяет соседей при ошибке
func goFetchSlice[T any](g *errgroup.Group, parentCtx context.Context, logger *slog.Logger, name string, timeOut time.Duration, fn sliceFetchFn[T]) {
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(parentCtx, timeOut)
		defer cancel()

		start := time.Now()
		data, err := fn(ctx)
		if err != nil {
			logger.Info(name+" NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // важный момент: не «роняем» группу, остальные задачи продолжат работу
		}
		logger.Info(name+" fetched",
			slog.Int("count", len(data)),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug(name+" data:", " ", data)
		return nil
	})
}

// аналог для «неслайсовых» результатов (например, сводка/структура)
func goFetchValue(g *errgroup.Group, parentCtx context.Context, logger *slog.Logger, name string, timeOut time.Duration, fn func(ctx context.Context) (any, error)) {
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(parentCtx, timeOut)
		defer cancel()

		start := time.Now()
		val, err := fn(ctx)
		if err != nil {
			logger.Info(name+" NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil
		}
		logger.Info(name+" fetched", slog.Duration("dur", time.Since(start)))
		logger.Debug(name+" data:", " ", val)
		return nil
	})
}

// run — «бизнес-логика», умеет останавливаться по ctx.Done().
func run(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) error {
	var (
		rs model.ResultSetT
		mu sync.Mutex
	)
	parentCtx := ctx // родительский ctx живёт до SIGINT/SIGTERM

	// 1) один http.Client на весь процесс (reuse пула соединений)
	client := &http.Client{Timeout: 5 * time.Second}

	// 2) конструируем сервисы с контекстным Fetch
	//svcMms := mms.NewService(logger, cfg, client)
	svcSupp := support.NewService(logger, cfg, client)
	svcInc := incidentdata.NewService(logger, cfg, client)

	// 3) errgroup с лимитом параллелизма
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(7) // лимит активных горутин -- TODO: или использовать pool?

	perReqTimeout := 3 * time.Second

	// 4) параллельные задачи (каждая – короткая обёртка)
	sms.GoFetch(g, ctx, logger, perReqTimeout, cfg, &rs, &mu) //с одной стороны вызов проще, но аргументов уже много - и не особо наглядно
	// goFetchSlice(g, ctx, logger, "sms", perReqTimeout, func(ctx context.Context) ([]sms.SMSData, error) {
	// 	return sms.Fetch(logger, cfg)
	// })
	voice.GoFetch(g, ctx, logger, perReqTimeout, cfg, &rs, &mu)
	// goFetchSlice(g, ctx, logger, "voice", perReqTimeout, func(ctx context.Context) ([]voicedata.VoiceCallData, error) {
	// 	return voicedata.Fetch(logger, cfg)
	// })

	email.GoFetch(g, ctx, logger, perReqTimeout, cfg, &rs, &mu)
	// goFetchSlice(g, ctx, logger, "email", perReqTimeout, func(ctx context.Context) ([]emaildata.EmailData, error) {
	// 	return emaildata.Fetch(logger, cfg)
	// })
	goFetchValue(g, ctx, logger, "billing", perReqTimeout, func(ctx context.Context) (any, error) {
		return billingstat.Fetch(logger, cfg) // вернёт структуру/сводку
	})

	mms.GoFetch(g, ctx, logger, perReqTimeout, client, cfg, &rs, &mu)
	// goFetchSlice(g, ctx, logger, "mms", perReqTimeout, func(ctx context.Context) ([]mms.MMSData, error) {
	// 	return svcMms.Fetch(ctx)
	// })
	goFetchSlice(g, ctx, logger, "support", perReqTimeout, func(ctx context.Context) ([]support.SupportData, error) {
		return svcSupp.Fetch(ctx)
	})
	goFetchSlice(g, ctx, logger, "incident", perReqTimeout, func(ctx context.Context) ([]incidentdata.IncidentData, error) {
		return svcInc.Fetch(ctx)
	})

	// 5) ждём завершения всех «первичных» фетчей
	_ = g.Wait()

	// 6) heartbeat + graceful shutdown
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Эмуляция длительной работы с периодической проверкой контекста.
	for {
		select {
		case <-parentCtx.Done():
			logger.Debug("state_Collector.run(): ctx cancelled — graceful exit")
			return nil
		case t := <-ticker.C:
			logger.Debug("state_Collector.heartbeat", slog.Time("ts", t))
		}
	}

}

// логирование - setupLogger строит slog.Logger согласно конфигурации.
func setupLogger(cfg LogCfg) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(cfg.Level)); err != nil { // преобразование текстового флага (например, -log.level=debug) в специальный тип slog.Level
		lvl = slog.LevelInfo // slog.LevelInfo при ошибке

		// Уведомляем пользователя о проблеме флагов
		slog.Warn("Invalid log level provided, falling back to 'info'",
			slog.String("provided_level", cfg.Level),
		)
	}

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true, // покажет файл:строку
	}

	var h slog.Handler
	switch cfg.Format {
	case "json":
		h = slog.NewJSONHandler(os.Stdout, opts)
	default:
		h = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(h)
}
