package smsdata

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	countries "main/internal/alpha2"
	m "main/internal/model"
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
	groupCtx context.Context,
	logger *slog.Logger,
	timeout time.Duration,
	cfg *config.CfgApp,
	rs *m.ResultSetT,
	mu *sync.Mutex,
) {
	g.Go(func() error {
		// таймаут на задачу
		ctx := groupCtx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(groupCtx, timeout)
			defer cancel()
		}

		start := time.Now()

		nonSortedData, err := Fetch(ctx, logger, cfg) // []sms.SMSData
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("sms cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("sms NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("sms cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}
		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedSMS(nonSortedData) // [][]sms.SMSData

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.SMS = sortedData
		mu.Unlock()

		// посчитать реальное количество строк во всех под-срезах
		total := 0
		for _, part := range sortedData {
			total += len(part)
		}

		logger.Info("sms fetched",
			slog.Int("count", total),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("sms data:", " ", sortedData)
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
func BuildSortedSMS(in []m.SMSData) [][]m.SMSData {
	// 1) нормализуем страны (делаем копию входного среза)
	mapped := make([]m.SMSData, len(in))
	copy(mapped, in)
	for i := range mapped {
		mapped[i].Country = countries.CountryName(mapped[i].Country)
	}

	// 2) сортировка по провайдеру (A→Z)
	byProvider := make([]m.SMSData, len(mapped))
	copy(byProvider, mapped)

	slices.SortStableFunc(byProvider, func(a, b m.SMSData) int {
		return strings.Compare(a.Provider, b.Provider) //не учитывал strings.ToLower, может и стоит
	})

	// 3) сортировка по стране (A→Z)
	byCountry := make([]m.SMSData, len(mapped))
	copy(byCountry, mapped)

	slices.SortStableFunc(byCountry, func(a, b m.SMSData) int {
		return strings.Compare(a.Country, b.Country)
	})

	// 4) объединяем
	return [][]m.SMSData{byProvider, byCountry}
}
