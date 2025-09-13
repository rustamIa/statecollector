package billingstat

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

var fetchBills = Fetch //чтобы мокнуть ф-ию в тестах

// аналог для «неслайсовых» результатов (например, сводка/структура)
func GoFetch(
	g *errgroup.Group,
	parentCtx context.Context,
	logger *slog.Logger,
	timeout time.Duration,
	cfg *config.CfgApp,
	rs *m.ResultSetT,
	mu *sync.Mutex,
) {
	g.Go(func() error {
		ctx := parentCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parentCtx, timeout)
			defer cancel()
		}

		start := time.Now()
		data, err := fetchBills(ctx, logger, cfg)

		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("billing cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("billing NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("billing cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.Billing = data
		mu.Unlock()

		logger.Info("billing fetched",
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("billing data:", " ", data)
		return nil

		// if err != nil {
		// 	logger.Info(name+" NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
		// 	return nil
		// }
		// logger.Info(name+" fetched", slog.Duration("dur", time.Since(start)))
		// logger.Debug(name+" data:", " ", val)
		// return nil
	})
}
