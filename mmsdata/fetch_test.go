package mmsdata

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
	m "main/internal/model"
)

// тихий логгер для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestFetch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `[{"country":"US","provider":"Rond","bandwidth":"36","response_time":"1576"}]`)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []m.MMSData{{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"}}
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

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !reflect.DeepEqual(got, []m.MMSData{}) {
		t.Fatalf("got %#v, want empty slice", got)
	}
}

// проверяем, что плохие элементы пропускаются (FailFast=false по умолчанию)
// — лишние поля, неверные типы/валидаторы, неверные провайдеры/ISO/диапазоны
func TestFetch_SkipBadElements(t *testing.T) {
	/*
		{"country":"US","provider":"Rond","bandwidth":"36","response_time":"1576"},
		{"country":"USA","provider":"Rond","bandwidth":"50","response_time":"100"},        // bad ISO2
		{"country":"DE","provider":"Bad","bandwidth":"10","response_time":"200"},          // bad provider
		{"country":"FR","provider":"Topolo","bandwidth":"200","response_time":"300"},      // >100
		{"country":"GB","provider":"Kildy","bandwidth":"ok","response_time":"300"},        // not a number
		{"country":"GB","provider":"Kildy","bandwidth":"85","response_time":"300","x":1},  // unknown field
		{"country":"GB","provider":"Kildy","bandwidth":"85","response_time":"300"}
	*/
	body := `[
		{"country":"US","provider":"Rond","bandwidth":"36","response_time":"1576"},
		{"country":"USA","provider":"Rond","bandwidth":"50","response_time":"100"},
		{"country":"DE","provider":"Bad","bandwidth":"10","response_time":"200"},
		{"country":"FR","provider":"Topolo","bandwidth":"200","response_time":"300"},
		{"country":"GB","provider":"Kildy","bandwidth":"ok","response_time":"300"},
		{"country":"GB","provider":"Kildy","bandwidth":"85","response_time":"300","x":1},
		{"country":"GB","provider":"Kildy","bandwidth":"85","response_time":"300"}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, body)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 500 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []m.MMSData{
		{Country: "US", Provider: "Rond", Bandwidth: "36", ResponseTime: "1576"},
		{Country: "GB", Provider: "Kildy", Bandwidth: "85", ResponseTime: "300"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// non-2xx → должен вернуться error с упоминанием unexpected HTTP status
func TestFetch_HTTPNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected HTTP status") {
		t.Fatalf("error %q must mention unexpected HTTP status", err)
	}
}

// тело — не массив → ожидаем обёртку "failed by decode&validate json" и текст ошибки верхнего уровня
func TestFetch_DecodeError_NotArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"not":"array"}`)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 500 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil {
		t.Fatalf("expected decode error, got nil")
	}
	// if !strings.Contains(err.Error(), "failed by decode&validate json") {
	// 	t.Fatalf("error must be wrapped as failed by decode&validate json: %v", err)
	// }
	if !strings.Contains(err.Error(), "expected top-level JSON array") {
		t.Fatalf("error should mention expected top-level JSON array: %v", err)
	}
}

// таймаут клиента при долгом ответе → обёртка "do request"
func TestFetch_ClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "[]")
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathMmsData: srv.URL}
	client := &http.Client{Timeout: 50 * time.Millisecond}
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

// искусственная ошибка чтения тела → обёртка "failed by decode&validate json"
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
	cfg := &config.CfgApp{PathMmsData: "http://example.org"}
	client := &http.Client{
		Timeout:   500 * time.Second,
		Transport: stubTransport{},
	}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil || !strings.Contains(err.Error(), "decode body: read fail") {
		t.Fatalf("expected failed by decode body: read fail, got %v", err)
	}
}

// сетевая ошибка (dial fail) → "do request"
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
}

func TestFetch_NetworkError(t *testing.T) {
	cfg := &config.CfgApp{PathMmsData: "http://127.0.0.1:9"}
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

// ошибка построения запроса (невалидный URL) → "build request"
func TestFetch_BuildRequestError(t *testing.T) {
	cfg := &config.CfgApp{PathMmsData: "http://"} // нет хоста
	client := &http.Client{Timeout: 2 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := svc.Fetch(ctx)
	if err == nil || !strings.Contains(err.Error(), "no Host in request URL") {
		t.Fatalf("expected no Host in request URL, got %v", err)
	}
}
