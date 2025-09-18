// httpserver/httpserver_test.go
package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"main/config"

	m "main/internal/model"
)

/*
	var SmsTest1 = [][]model.SMSData{
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

	var mmsTest1 = [][]m.MMSData{
		{
			{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"},
		},
		{
			{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},
		},
	}

	var voiceTest1 = []m.VoiceCallData{
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

	var emailTest1 = map[string][][]m.EmailData{
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

var billTest1 = m.BillingData{CreateCustomer: true, Purchase: false, Payout: true, Recurring: false, FraudControl: true, CheckoutPage: false}
var suppTest1 = m.SupportData{Topic: "issue of everything", ActiveTickets: 1}
var incidTest1 = m.IncidentData{Topic: "boom", Status: "active"}
*/
func nonZeroBilling() m.BillingData {
	return m.BillingData{CreateCustomer: true,
		Purchase:     true,
		Payout:       true,
		Recurring:    true,
		FraudControl: true,
		CheckoutPage: true}
}

func TestHandler_HappyPath(t *testing.T) {
	// подменяем fetch
	orig := fetch
	t.Cleanup(func() { fetch = orig })
	fetch = func(ctx context.Context, _ *slog.Logger, _ *config.CfgApp) (m.ResultSetT, m.ResultT) {
		return m.ResultSetT{ //данные квази-пустые, len покажет = 1
			SMS:       [][]m.SMSData{[]m.SMSData{m.SMSData{}}},
			MMS:       [][]m.MMSData{[]m.MMSData{m.MMSData{}}},
			VoiceCall: []m.VoiceCallData{m.VoiceCallData{}},
			Email:     map[string][][]m.EmailData{"ru": [][]m.EmailData{[]m.EmailData{m.EmailData{}}}},
			Billing:   nonZeroBilling(),
			Support:   []int{1},
			Incidents: []m.IncidentData{m.IncidentData{}},
		}, m.ResultT{}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	makeHandleConnection(nil, &config.CfgApp{})(rr, req) //(rr, req) — это моментальный вызов возвращённой функции с аргументами: rr — httptest.NewRecorder(), это *httptest.ResponseRecorder, который реализует http.ResponseWriter
	/*нагляднее так
	  h := makeHandleConnection(nil, &config.CfgApp{}) // h имеет тип http.HandlerFunc
	  h(rr, req)                                       // вызываем обработчик напрямую
	  // или так:
	  h.ServeHTTP(rr, req) // у http.HandlerFunc есть метод ServeHTTP, он вызывает саму функцию
	*/
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var payload struct {
		ResultSet json.RawMessage `json:"resultSet"`
		Result    json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("bad json: %v", err)
	}
}

func TestHandler_ContextCanceled_NoWrite(t *testing.T) {
	orig := fetch
	t.Cleanup(func() { fetch = orig })

	var entered atomic.Bool
	// имитируем долгий сбор: ждём отмены ctx
	fetch = func(ctx context.Context, _ *slog.Logger, _ *config.CfgApp) (m.ResultSetT, m.ResultT) {
		entered.Store(true)
		<-ctx.Done()
		return m.ResultSetT{}, m.ResultT{}
	}

	rr := httptest.NewRecorder()
	parentCtx, cancel := context.WithCancel(context.Background())
	// делаем запрос с уже отменённым контекстом
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parentCtx)

	makeHandleConnection(nil, &config.CfgApp{})(rr, req)

	// хендлер должен вернуться, ничего не записав
	if rr.Body.Len() != 0 {
		t.Fatalf("expected no body on canceled ctx, got: %q", rr.Body.String())
	}
	if !entered.Load() { //тип проверили что fetch изнутри makeHandleConnection
		t.Fatalf("fetch wasn't called")
	}
}

func TestHttpServer_GracefulShutdown(t *testing.T) {
	// создаем сервер, потом serveOnListener, потом стартует клиент, и cancel
	// Мокаем fetch, чтобы хендлер «подумал»
	orig := fetch
	t.Cleanup(func() { fetch = orig })
	fetch = func(ctx context.Context, _ *slog.Logger, _ *config.CfgApp) (m.ResultSetT, m.ResultT) {
		time.Sleep(100 * time.Millisecond)
		return m.ResultSetT{}, m.ResultT{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := "http://" + ln.Addr().String() + "/"

	done := make(chan error, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	go func() { done <- serveOnListener(ctx, logger, &config.CfgApp{}, ln) }()

	// Запускаем запрос
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, addr, nil)
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	// Даем запросу войти в хендлер и инициируем shutdown
	time.Sleep(20 * time.Millisecond)
	cancel() //стоп основного контекста

	// 1) Запрос должен завершиться (graceful)
	select {
	case err := <-errCh:
		t.Fatalf("request error: %v", err)
	case resp := <-respCh:
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request didn't finish under graceful shutdown")
	}

	// 2) Сервер должен завершиться
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shutdown gracefully")
	}
}
