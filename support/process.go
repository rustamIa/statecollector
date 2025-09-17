package support

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"math"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type supportFetcher interface {
	Fetch(ctx context.Context) ([]m.SupportData, error)
}

var newService = func(logger *slog.Logger, cfg *config.CfgApp, client *http.Client) supportFetcher {
	return NewService(logger, cfg, client)
}

// для примера сделан отдельный goFetch - вызов такого будет занимать в RUN меньше места, хотя да он схож шаблону goFetchSlice
// контекст в GoFetch нужен не для mu.Unlock(), а для ранней отмены/таймаута самой работы, чтобы g.Wait() не завис навсегда, если GoFetchSMS подвис
func GoFetch(
	g *errgroup.Group,
	parentCtx context.Context,
	logger *slog.Logger,
	timeout time.Duration,
	client *http.Client,
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

		s := newService(logger, cfg, client) // будем мокать, поэтому через интерфейс

		nonSortedData, err := s.Fetch(ctx)
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("support cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("support NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу errgroup, если тут будет err, то завершаться все горутины errgroup
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("support cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}

		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedSupport(nonSortedData)

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.Support = sortedData
		mu.Unlock()

		logger.Info("support fetched",
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("support data:", "loadLevel=", sortedData[0], " waitMinutes=", sortedData[1])
		return nil
	})
}

// BuildSortedSupport считает интегральную нагрузку саппорта и потенциальное время ожидания.
// Возвращает []int{loadLevel, waitMinutes}:
//
//	loadLevel: 1 (<9 тикетов), 2 (9..16), 3 (>16)
//	waitMinutes: потенциальное время ожидания ответа на новый тикет (минуты)
func BuildSortedSupport(data []m.SupportData) []int {
	const teamThroughputPerHour = 18.0
	const minutesPerTicket = 60.0 / teamThroughputPerHour // ~3.33 мин/тикет (вся команда)

	// суммируем только валидные значения
	totalOpen := 0
	for _, d := range data {
		if d.ActiveTickets > 0 {
			totalOpen += d.ActiveTickets
		}
	}

	// потенциальное время ожидания
	waitMinutes := int(math.Ceil(float64(totalOpen) * minutesPerTicket))

	// уровни нагрузки по количеству открытых тикетов
	loadLevel := 0
	switch {
	case totalOpen < 9:
		loadLevel = 1
	case totalOpen <= 16:
		loadLevel = 2
	default:
		loadLevel = 3
	}

	return []int{loadLevel, waitMinutes}
}
