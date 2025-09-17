package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"log/slog"
	"time"

	//менеджер горутин удобен, когда нужно запустить несколько задач параллельно, дождаться их завершения и аккуратно обойтись с ошибками и отменой по контексту.
	"main/config"
	s "main/internal/httpserver"
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

	// Главная работа сервиса.
	if err := run(ctx, logger, cfgApp); err != nil {
		logger.Error("collector failed", slog.Any("err", err))
	}

	logger.Info("state_Collector stopped")
}

// run — «бизнес-логика», умеет останавливаться по ctx.Done().
func run(parentCtx context.Context, logger *slog.Logger, cfg *config.CfgApp) error {

	s.HttpServer(parentCtx, logger, cfg)

	//heartbeat + graceful shutdown
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
