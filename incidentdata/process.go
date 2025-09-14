package incidentdata

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type supportIncidenter interface {
	Fetch(ctx context.Context) ([]m.IncidentData, error)
}

var newService = func(logger *slog.Logger, cfg *config.CfgApp, client *http.Client) supportIncidenter {
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
				logger.Info("Incidents cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("Incidents NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("Incidents cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}

		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedIncident(nonSortedData)

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.Incidents = sortedData
		mu.Unlock()

		logger.Info("Incidents fetched",
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("Incidents data:", " ", sortedData)
		return nil
	})
}

// BuildSortedIncident сортирует все инциденты, чтобы все со статусом active оказались наверху списка
func BuildSortedIncident(data []m.IncidentData) []m.IncidentData {
	if len(data) == 0 {
		return nil
	}
	out := make([]m.IncidentData, 0, len(data))

	// сначала активные
	for _, d := range data {
		if d.Status == "active" {
			out = append(out, d)
		}
	}
	// затем все остальные
	for _, d := range data {
		if d.Status != "active" {
			out = append(out, d)
		}
	}

	return out
}
