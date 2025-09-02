package incidentdata

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"main/config"
)

// тихий логгер для тестов
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestFetch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `[{"topic":"Billing isn’t allowed in US","status":"closed"}]`)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathIncidentData: srv.URL}
	client := &http.Client{Timeout: 500 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []IncidentData{{Topic: "Billing isn’t allowed in US", Status: "closed"}}
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

	cfg := &config.CfgApp{PathIncidentData: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !reflect.DeepEqual(got, []IncidentData{}) {
		t.Fatalf("got %#v, want empty slice", got)
	}
}

// проверяем, что плохие элементы пропускаются (FailFast=false по умолчанию)
// — лишние поля, неверные типы/валидаторы, неверные провайдеры/ISO/диапазоны
func TestFetch_SkipBadElements(t *testing.T) {
	body := `[
		{"topic":"Billing isn’t allowed in US","status":"closed"},
		{"topic":"Wrong SMS delivery time","status":"active"},
		{"country":"GB","provider":"Kildy","bandwidth":"85","response_time":"300"}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, body)
	}))
	defer srv.Close()

	cfg := &config.CfgApp{PathIncidentData: srv.URL}
	client := &http.Client{Timeout: 500 * time.Second}
	svc := NewService(testLogger(), cfg, client)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	got, err := svc.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	want := []IncidentData{
		{Topic: "Billing isn’t allowed in US", Status: "closed"},
		{Topic: "Wrong SMS delivery time", Status: "active"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
