package emaildata

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

var fetchEmails = Fetch

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

		nonSortedData, err := fetchEmails(ctx, logger, cfg)
		if err != nil {
			// отличаем отмену от реальной ошибки
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info("email cancelled", slog.Duration("dur", time.Since(start)))
				return nil
			}
			logger.Info("email NOT fetched", slog.Any("err", err), slog.Duration("dur", time.Since(start)))
			return nil // не валим группу
		}

		// перед публикацией ещё раз убеждаемся, что не отменено
		select {
		case <-ctx.Done():
			logger.Info("email cancelled before publish", slog.Duration("dur", time.Since(start)))
			return nil
		default:
		}
		//все что ниже продолжит выполнение как по default
		sortedData := BuildSortedEmails(nonSortedData)

		// сохранить результат с защитой от гонок
		mu.Lock()
		rs.Email = sortedData
		mu.Unlock()

		// посчитать реальное количество строк во всех под-срезах
		total := 0
		for _, part := range sortedData {
			total += len(part)
		}

		logger.Info("email fetched",
			slog.Int("count", total),
			slog.Duration("dur", time.Since(start)),
		)
		logger.Debug("email data:", " ", sortedData)
		return nil
	})
}

// BuildSortedEmails группирует по стране и провайдеру, считает средний DeliveryTime,
// и для каждой страны возвращает [0] — топ-3 самых быстрых, [1] — топ-3 самых медленных.
func BuildSortedEmails(in []m.EmailData) map[string][][]m.EmailData {
	// country -> provider -> (sum, count)
	type agg struct {
		sum   int
		count int
	}

	byCountryProv := make(map[string]map[string]agg, 64)

	for _, e := range in {
		c := strings.ToUpper(strings.TrimSpace(e.Country)) // в Fetch уже валидация alpha-2, просто нормализуем регистр
		if _, ok := byCountryProv[c]; !ok {                //проверяем, есть ли уже внутренняя карта для страны c
			byCountryProv[c] = make(map[string]agg, 8)
		}
		a := byCountryProv[c][e.Provider] //если ключа e.Provider ещё нет, индексирование карты возвращает нулевое значение типа agg, т.е. agg{sum:0, count:0}
		a.sum += e.DeliveryTime
		a.count++
		byCountryProv[c][e.Provider] = a
	}

	out := make(map[string][][]m.EmailData, len(byCountryProv))

	for country, provAgg := range byCountryProv {
		// Собираем усреднённые записи по провайдерам этой страны
		avgList := make([]m.EmailData, 0, len(provAgg))
		for provider, a := range provAgg {
			avg := int(math.Round(float64(a.sum) / float64(a.count)))
			avgList = append(avgList, m.EmailData{
				Country:      country,
				Provider:     provider,
				DeliveryTime: avg,
			})
		}

		// Сортируем по среднему времени (меньше = быстрее), при равенстве — по имени провайдера
		slices.SortStableFunc(avgList, func(a, b m.EmailData) int { //странная ф-ия сортировки
			if a.DeliveryTime < b.DeliveryTime {
				return -1
			}
			if a.DeliveryTime > b.DeliveryTime {
				return 1
			}
			return strings.Compare(a.Provider, b.Provider)
		})

		// Топ-3 быстрых
		n := 3
		if len(avgList) < n {
			n = len(avgList)
		}
		fast := make([]m.EmailData, n)
		copy(fast, avgList[:n])

		// Топ-3 медленных (с конца)
		mn := 3
		if len(avgList) < mn {
			mn = len(avgList)
		}
		slow := make([]m.EmailData, mn)
		// Берём последние mn по возрастанию -> это самые медленные
		copy(slow, avgList[len(avgList)-mn:])

		out[country] = [][]m.EmailData{fast, slow}
	}

	return out
}
