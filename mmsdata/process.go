package mmsdata

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	countries "main/internal/alpha2"
	m "main/internal/model"
	"net/http"
	"slices"
	"strings"
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

		s := NewService(logger, cfg, client)

		nonSortedData, err := s.Fetch(ctx)
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("mms cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("mms NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("mms cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}

		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedMMS(nonSortedData) // [][]MMSData

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.MMS = sortedData
		mu.Unlock()

		// посчитать реальное количество строк во всех под-срезах
		total := 0
		for _, part := range sortedData {
			total += len(part)
		}

		logger.Info("mms fetched",
			slog.Int("count", total),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("mms data:", " ", sortedData)
		return nil
	})
}

// BuildSortedSMS:
// 1) подменяет Country: alpha-2 → полное название,
// 2) готовит два набора:
//   - по провайдеру A→Z,
//   - по стране A→Z,
//
// 3) объединяет в [][]SMSData, где [0] — сортировка по провайдеру, [1] — по стране.
//
// ВАЖНО: валидацию вы уже прошли в Fetch (там Country — alpha-2).
// После подмены на полные названия повторно Validate() вызывать не нужно.
func BuildSortedMMS(in []m.MMSData) [][]m.MMSData {
	// 1) нормализуем страны (делаем копию входного среза)
	mapped := make([]m.MMSData, len(in))
	copy(mapped, in)
	for i := range mapped {
		mapped[i].Country = countries.CountryName(mapped[i].Country)
	}

	// 2) сортировка по провайдеру (A→Z)
	byProvider := make([]m.MMSData, len(mapped))
	copy(byProvider, mapped)

	slices.SortStableFunc(byProvider, func(a, b m.MMSData) int {
		return strings.Compare(a.Provider, b.Provider) //не учитывал strings.ToLower, может и стоит
	})

	// 3) сортировка по стране (A→Z)
	byCountry := make([]m.MMSData, len(mapped))
	copy(byCountry, mapped)

	slices.SortStableFunc(byCountry, func(a, b m.MMSData) int {
		return strings.Compare(a.Country, b.Country)
	})

	// 4) объединяем
	return [][]m.MMSData{byProvider, byCountry}
}
