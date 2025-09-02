package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"log/slog"
	"time"

	//----sms "main/smsdata"

	"main/billingstat"
	"main/config"
	"main/emaildata"
	incident "main/incedentdata"
	mms "main/mmsdata"
	sms "main/smsdata"
	support "main/support"
	"main/voicedata"
)

//TODO тянуть конфигурацию для sms.Get и mms.Get из файла config

// LogCfg описывает параметры логирования,
// которые удобнее всего задавать флагами/ENV.
type LogCfg struct {
	Format string // text | json
	Level  string // debug | info | warn | error
}

// readLogCfg парсит флаги командной строки и возвращает конфиг логгера.
// запукать в терминале bash, но не power shell
//
//	$ go run . -log.format=json -log.level=debug
func readLogCfg() LogCfg {
	var cfg LogCfg
	flag.StringVar(&cfg.Format, "log.format", "text", "log output format: text|json")
	flag.StringVar(&cfg.Level, "log.level", "info", "log level: debug|info|warn|error")
	flag.Parse()
	return cfg
}

// логирование
// setupLogger creates a new slog.Logger instance with a text output
// handler writing to os.Stdout at the debug level
/*func setupLogger() *slog.Logger {
	var log *slog.Logger
	log = slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	return log
}
*/
// setupLogger строит slog.Logger согласно конфигурации.
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

	// Главная работа сервиса.
	if err := run(ctx, logger, cfgApp); err != nil {
		logger.Error("collector failed", slog.Any("err", err))
	}

	logger.Info("state_Collector stopped")
}

// run — «бизнес-логика», умеет останавливаться по ctx.Done().
func run(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) error {

	smsData, err := sms.Fetch(logger, cfg)
	if err != nil {
		//fmt.Errorf("sms: %w", err) - этот msg уже выдал sms.get
		logger.Info("sms data NOT fetched")
	} else {
		logger.Info("sms data fetched",
			slog.Int("count", len(smsData)),
		)
		logger.Debug("sms data:",
			" ", smsData,
		)
	}

	VoiceData, err := voicedata.Fetch(logger, cfg)
	if err != nil {
		//fmt.Errorf("sms: %w", err) - этот msg уже выдал sms.get
		logger.Info("vioce data NOT fetched")
	} else {
		logger.Info("vioce data fetched",
			slog.Int("count", len(VoiceData)),
		)
		logger.Debug("Voice data:",
			" ", VoiceData,
		)
	}

	EmailData, err := emaildata.Fetch(logger, cfg)
	if err != nil {
		//fmt.Errorf("sms: %w", err) - этот msg уже выдал sms.get
		logger.Info("email data NOT fetched")
	} else {
		logger.Info("email data fetched",
			slog.Int("count", len(EmailData)),
		)
		logger.Debug("Email data:",
			" ", EmailData,
		)
	}

	BillingState, err := billingstat.Fetch(logger, cfg)
	if err != nil {
		logger.Info("billing state NOT fetched")
	} else {
		logger.Info("billing state fetched")
		logger.Debug("billing state data:",
			" ", BillingState,
		)
	}

	// HTTP клиент
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	svcMms := mms.NewService(logger, cfg, client)

	if got, err := svcMms.Fetch(ctx); err != nil {
		logger.Info("mms data NOT fetched")
	} else {
		logger.Info("mms data fetched",
			slog.Int("count", len(got)),
		)
		logger.Debug("mms data:",
			" ", got,
		)
	}

	svcSupp := support.NewService(logger, cfg, client)
	// Вызов Fetch
	if got, err := svcSupp.Fetch(ctx); err != nil {
		logger.Info("failed to fetch Support data")
	} else {
		logger.Info("Support data fetched",
			slog.Int("count", len(got)),
		)
		logger.Debug("Support data:",
			" ", got,
		)
	}

	svcIncidents := incident.NewService(logger, cfg, client)
	// Вызов Fetch
	if got, err := svcIncidents.Fetch(ctx); err != nil {
		logger.Info("failed to fetch incident data")
	} else {
		logger.Info("incident data fetched",
			slog.Int("count", len(got)),
		)
		logger.Debug("incident data:",
			" ", got,
		)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Эмуляция длительной работы с периодической проверкой контекста.
	for {
		select {
		case <-ctx.Done():
			logger.Debug("state_Collector.run(): ctx cancelled — graceful exit")
			return nil
		case t := <-ticker.C:
			logger.Debug("state_Collector.heartbeat", slog.Time("ts", t))
		}
	}

}
