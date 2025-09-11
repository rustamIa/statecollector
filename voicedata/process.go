package voicedata

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

// для примера сделан отдельный goFetch - вызов такого будет занимать в RUN меньше места, хотя да он схож шаблону goFetchSlice
// контекст в GoFetch нужен не для mu.Unlock(), а для ранней отмены/таймаута самой работы, чтобы g.Wait() не завис навсегда, если GoFetchSMS подвис
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
		// таймаут на задачу
		ctx := parentCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(parentCtx, timeout)
			defer cancel()
		}

		start := time.Now()

		data, err := Fetch(ctx, logger, cfg) // []VoiceCallData
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("voice cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("voice NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("voice cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.VoiceCall = data
		mu.Unlock()

		logger.Info("voice fetched",
			slog.Int("count", len(data)),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("voice data:", " ", data)
		return nil
	})
}
