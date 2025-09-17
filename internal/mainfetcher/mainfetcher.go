package mainfetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"main/config"
	"net/http"
	"reflect"
	"sync"
	"time"

	bill "main/billingstat"
	email "main/emaildata"
	incident "main/incidentdata"
	m "main/internal/model"
	mms "main/mmsdata"
	sms "main/smsdata"
	"main/support"
	voice "main/voicedata"

	"golang.org/x/sync/errgroup"
)

func PrepaireResStub() {

	smsTest1 := [][]m.SMSData{
		{
			{Country: "US", Bandwidth: "64", ResponseTime: "1923", Provider: "Rond"},
		},
		{
			{Country: "GB", Bandwidth: "88", ResponseTime: "1892", Provider: "Topolo"},
		},
		{
			{Country: "FR", Bandwidth: "61", ResponseTime: "170", Provider: "Topolo"},
		},
		{
			{Country: "BL", Bandwidth: "57", ResponseTime: "267", Provider: "Kildy"},
		},
	}
	mmsTest1 := [][]m.MMSData{
		{
			{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"},
		},
		{
			{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},
		},
	}
	voiceTest1 := []m.VoiceCallData{
		{
			Country:             "RU",
			Bandwidth:           "86",
			ResponseTime:        "297",
			Provider:            "TransparentCalls",
			ConnectionStability: 0.9,
			TTFB:                120,
			VoicePurity:         80,
			MedianOfCallsTime:   30,
		},
		{
			Country:             "US",
			Bandwidth:           "64",
			ResponseTime:        "1923",
			Provider:            "E-Voice",
			ConnectionStability: 0.75,
			TTFB:                200,
			VoicePurity:         65,
			MedianOfCallsTime:   45,
		},
		{
			Country:             "GB",
			Bandwidth:           "88",
			ResponseTime:        "1892",
			Provider:            "JustPhone",
			ConnectionStability: 0.6,
			TTFB:                150,
			VoicePurity:         70,
			MedianOfCallsTime:   50,
		},
	}

	emailTest1 := map[string][][]m.EmailData{
		"RU": {
			{
				{Country: "RU", Provider: "Gmail", DeliveryTime: 23},
			},
			{
				{Country: "RU", Provider: "Yahoo", DeliveryTime: 169},
			},
			{
				{Country: "RU", Provider: "Hotmail", DeliveryTime: 63},
			},
			{
				{Country: "RU", Provider: "MSN", DeliveryTime: 475},
			},
			{
				{Country: "RU", Provider: "Orange", DeliveryTime: 519},
			},
			{
				{Country: "RU", Provider: "Comcast", DeliveryTime: 408},
			},
			{
				{Country: "RU", Provider: "AOL", DeliveryTime: 254},
			},
			{
				{Country: "RU", Provider: "GMX", DeliveryTime: 246},
			},
		},
	}
	/*	billTest1 := bill.BillingData{CreateCustomer: true, Purchase: false, Payout: true, Recurring: false, FraudControl: true, CheckoutPage: false}
		suppTest1 := supp.SupportData{Topic: "issue of everything", ActiveTickets: 1}
		incidTest1 := incid.IncidentData{Topic: "boom", Status: "active"}*/

	// 2. Создание и заполнение структуры ResultSetT
	resultSet := m.ResultSetT{
		SMS:       smsTest1,
		MMS:       mmsTest1,
		VoiceCall: voiceTest1,
		Email:     emailTest1,
		//	Billing:   billingData,
		//		Support:   supportData,
		//		Incidents: incidentsData,
	}
	// 3. Создание и заполнение главной структуры ResultT
	/*result := ResultT{
		Status: true,
		Data:   resultSet,
		Error:  "",
	}*/

	// 4. Маршалинг всей структуры ResultT в JSON
	jsonData, err := json.Marshal(resultSet)
	if err != nil {
		log.Fatalf("Ошибка маршалинга: %v", err)
	}

	// 5. Вывод JSON в консоль
	fmt.Println(string(jsonData))

}

// -----------------------------------------
/* тип функции, возвращающей слайс данных
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
}*/
// goFetchSlice(g, ctx, logger, "sms", perReqTimeout, func(ctx context.Context) ([]sms.SMSData, error) {
// 	return sms.Fetch(logger, cfg)
// })
// goFetchSlice(g, ctx, logger, "voice", perReqTimeout, func(ctx context.Context) ([]voicedata.VoiceCallData, error) {
// 	return voicedata.Fetch(logger, cfg)
// })
// goFetchSlice(g, ctx, logger, "email", perReqTimeout, func(ctx context.Context) ([]emaildata.EmailData, error) {
// 	return emaildata.Fetch(logger, cfg)
// })
// goFetchSlice(g, ctx, logger, "mms", perReqTimeout, func(ctx context.Context) ([]mms.MMSData, error) {
// 	return svcMms.Fetch(ctx)
// })
// goFetchSlice(g, ctx, logger, "support", perReqTimeout, func(ctx context.Context) ([]support.SupportData, error) {
// 	return svcSupp.Fetch(ctx)
// })

type fetcher func(g *errgroup.Group, ctx context.Context) //обёртка-замыкание -- т.к. часть функций требует *http.Client, часть — нет, плюс передаются другие параметры

// «Связываем» конкретные GoFetch в единый тип, замыкая внешние зависимости
func makeFetcher(
	fn func(*errgroup.Group, context.Context, *slog.Logger, time.Duration, *config.CfgApp, *m.ResultSetT, *sync.Mutex),
	logger *slog.Logger, perReq time.Duration, cfg *config.CfgApp, rs *m.ResultSetT, mu *sync.Mutex,
) fetcher {
	return func(g *errgroup.Group, ctx context.Context) {
		fn(g, ctx, logger, perReq, cfg, rs, mu)
	}
}

func makeFetcherWithClient(
	fn func(*errgroup.Group, context.Context, *slog.Logger, time.Duration, *http.Client, *config.CfgApp, *m.ResultSetT, *sync.Mutex),
	logger *slog.Logger, perReq time.Duration, client *http.Client, cfg *config.CfgApp, rs *m.ResultSetT, mu *sync.Mutex,
) fetcher {
	return func(g *errgroup.Group, ctx context.Context) {
		fn(g, ctx, logger, perReq, client, cfg, rs, mu)
	}
}

func GetResultData(parentCtx context.Context, logger *slog.Logger, cfg *config.CfgApp, custom ...fetcher) (rs m.ResultSetT, r m.ResultT) {
	/*Наглядная «карта отмен»
	  SIGINT/SIGTERM  ─┐
	                   ├─(отменяет)→ parentCtx ──┐
	  manual stop()  ──┘                         ├─(отменяет)→ groupCtx (из errgroup), но сделано так чтобы горутина не возвращала ошибку, т.е. можно не использовать groupCtx для errgroup
	                              любая Go() вернула ошибку ──┘

	*/
	var mu sync.Mutex
	// 1) один http.Client на весь процесс (reuse пула соединений)
	client := &http.Client{Timeout: 5 * time.Second}

	// 2) конструируем сервисы с контекстным Fetch
	//svcMms := mms.NewService(logger, cfg, client)

	// 3) errgroup с лимитом параллелизма
	g, groupCtx := errgroup.WithContext(parentCtx)
	g.SetLimit(7) // лимит активных горутин -- TODO: или использовать pool - Для простого кейса лимита параллелизма SetLimit — идеально. Пул нужен, когда хочешь долгоживущих воркеров, очереди задач, приоритизацию и т.п.

	perReqTimeout := 3 * time.Second

	var fs []fetcher
	if len(custom) > 0 {
		fs = custom
	} else { //запуск Fetcher-ов по умолчанию корректный; custom для тестов
		fs = []fetcher{
			makeFetcher(sms.GoFetch, logger, perReqTimeout, cfg, &rs, &mu), //тут параметры - «чем и куда писать»
			makeFetcher(voice.GoFetch, logger, perReqTimeout, cfg, &rs, &mu),
			makeFetcher(email.GoFetch, logger, perReqTimeout, cfg, &rs, &mu),
			makeFetcherWithClient(mms.GoFetch, logger, perReqTimeout, client, cfg, &rs, &mu),
			makeFetcher(bill.GoFetch, logger, perReqTimeout, cfg, &rs, &mu),
			makeFetcherWithClient(support.GoFetch, logger, perReqTimeout, client, cfg, &rs, &mu),
			makeFetcherWithClient(incident.GoFetch, logger, perReqTimeout, client, cfg, &rs, &mu),
		}
	}

	// 4) параллельные задачи
	// Запускаем все задачи
	for _, f := range fs {
		f(g, groupCtx) //g и ctx — это про «когда и как бежать» - эти параметры нам нужны чтобы подменить в тестах
	}
	// sms.GoFetch(g, groupCtx, logger, perReqTimeout, cfg, &rs, &mu) //с одной стороны вызов проще, но аргументов уже много - и не особо наглядно

	// voice.GoFetch(g, groupCtx, logger, perReqTimeout, cfg, &rs, &mu)

	// email.GoFetch(g, groupCtx, logger, perReqTimeout, cfg, &rs, &mu)

	// mms.GoFetch(g, groupCtx, logger, perReqTimeout, client, cfg, &rs, &mu)

	// bill.GoFetch(g, groupCtx, logger, perReqTimeout, cfg, &rs, &mu)

	// support.GoFetch(g, groupCtx, logger, perReqTimeout, client, cfg, &rs, &mu)

	// incident.GoFetch(g, groupCtx, logger, perReqTimeout, client, cfg, &rs, &mu)

	// 5) ждём завершения всех фетчей
	_ = g.Wait()

	r = BuildResultT(rs)

	return rs, r

}

func validateResultSet(rs m.ResultSetT) error {
	// SMS
	if len(rs.SMS) == 0 {
		return fmt.Errorf("sms empty")
	}
	for _, batch := range rs.SMS {
		if len(batch) == 0 {
			return fmt.Errorf("sms has empty batch")
		}
	}

	// MMS
	if len(rs.MMS) == 0 {
		return fmt.Errorf("mms empty")
	}
	for _, batch := range rs.MMS {
		if len(batch) == 0 {
			return fmt.Errorf("mms has empty batch")
		}
	}

	// VoiceCall
	if len(rs.VoiceCall) == 0 {
		return fmt.Errorf("voice_call empty")
	}

	// Email
	if rs.Email == nil || len(rs.Email) == 0 {
		return fmt.Errorf("email empty")
	}
	for _, buckets := range rs.Email {
		if len(buckets) == 0 {
			return fmt.Errorf("email has empty buckets")
		}
		for _, bucket := range buckets {
			if len(bucket) == 0 {
				return fmt.Errorf("email has empty bucket")
			}
		}
	}

	// Billing — проверяем на нулевое значение структуры
	if reflect.ValueOf(rs.Billing).IsZero() {
		return fmt.Errorf("billing is zero")
	}

	// Support
	if len(rs.Support) == 0 {
		return fmt.Errorf("support empty")
	}

	// Incidents
	if len(rs.Incidents) == 0 {
		return fmt.Errorf("incident empty")
	}

	return nil
}

// BuildResult формирует r по заданным правилам
func BuildResultT(rs m.ResultSetT) m.ResultT {
	if err := validateResultSet(rs); err != nil {
		// есть пропуски
		return m.ResultT{
			Status: false,
			// Data не заполняем (останется нулевым значением)
			Error: "Error on collect data",
		}
	}

	// всё заполнено
	return m.ResultT{
		Status: true,
		Data:   rs,
		Error:  "",
	}
}
