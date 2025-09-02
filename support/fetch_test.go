package support

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"main/config"
)

// тихий логгер
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestFetch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `[{"topic":"SMS","active_tickets":13}]`)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []SupportData{{Topic: "SMS", ActiveTickets: 13}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestFetch_OK_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "[]")
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !reflect.DeepEqual(got, []SupportData{}) {
		t.Fatalf("got %#v, want empty slice", got)
	}
}

// элементы с лишними полями / невалидные должны пропускаться (FailFast=false по умолчанию)
func TestFetch_SkipBadElements(t *testing.T) {
	body := `[{"topic":"SMS","active_tickets":13},{"s":"MMS","active_tickets":"13"},{"topic":"OK","active_tickets":1}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, body)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []SupportData{
		{Topic: "SMS", ActiveTickets: 13},
		{Topic: "OK", ActiveTickets: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// non-2xx: тело дочитывается в Discard, декодер не вызывается, возвращается ошибка статуса
func TestFetch_HTTPNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected HTTP status") {
		t.Fatalf("error %q must mention unexpected HTTP status", err)
	}
}

// тело — не массив: ожидаем обёрнутую ошибку failed by decode&validate json с ErrTopLevelNotArray
func TestFetch_DecodeError_NotArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"topic":"SMS","active_tickets":13}`)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected failed by decode&validate json error, got nil")
	}
	//	if !strings.Contains(err.Error(), "failed by decode&validate json") {
	//		t.Fatalf("error must be wrapped as failed by decode&validate json: %v", err)
	//	}
	// полезно убедиться, что внутри действительно текст про ожидаемый массив
	if !strings.Contains(err.Error(), "expected top-level JSON array") {
		t.Fatalf("error should mention expected top-level JSON array: %v", err)
	}
}

// client.Timeout срабатывает при долгом ответе сервера → обёртка "do request"
func TestFetch_ContextTimeout_Client(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "[]")
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathSupportData: srv.URL}
	client := &http.Client{Timeout: 50 * time.Millisecond} // меньше задержки
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "do request") {
		t.Fatalf("error should be wrapped as do request: %v", err)
	}
}

// искусственная ошибка чтения тела (streaming decode должен упасть и быть обёрнут в "failed by decode&validate json")
type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("read fail") }
func (errReader) Close() error               { return nil }

type stubTransport struct{}

func (stubTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       errReader{},
		Header:     make(http.Header),
	}, nil
}

func TestFetch_ReadBodyError(t *testing.T) {
	cfg := &config.CfgApp{PathSupportData: "http://example.org"}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: stubTransport{},
	}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil || !strings.Contains(err.Error(), "decode body: read fail") {
		t.Fatalf("expected decode body: read fail, got %v", err)
	}
}

// сетевая ошибка (dial fail) → обёртка "do request"
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
}

func TestFetch_NetworkError(t *testing.T) {
	cfg := &config.CfgApp{PathSupportData: "http://127.0.0.1:9"}
	client := &http.Client{
		Timeout:   time.Second,
		Transport: failingTransport{},
	}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "do request") {
		t.Fatalf("error should be wrapped as do request: %v", err)
	}
}
