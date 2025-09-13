package billingstat

import (
	"context"
	"errors"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// безболезненный логгер
// func testLogger() *slog.Logger {
// 	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
// }

func TestGoFetch_SuccessPublishes(t *testing.T) {
	orig := fetchBills
	defer func() { fetchBills = orig }()

	want := m.BillingData{
		CreateCustomer: true,
		Purchase:       true,
		Payout:         false,
		Recurring:      true,
		FraudControl:   false,
		CheckoutPage:   true,
	}
	fetchBills = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (m.BillingData, error) {
		return want, nil
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	GoFetch(g, ctx, logger, 200*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("group returned error: %v", err)
	}

	mu.Lock()
	got := rs.Billing
	mu.Unlock()

	if got != want {
		t.Errorf("rs.Billing mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestGoFetch_FetchError_NoPublish(t *testing.T) {
	orig := fetchBills
	defer func() { fetchBills = orig }()

	fetchBills = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (m.BillingData, error) {
		return m.BillingData{}, errors.New("boom")
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	GoFetch(g, ctx, logger, 100*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("group returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if rs.Billing != (m.BillingData{}) {
		t.Errorf("rs.Billing should remain zero-value on fetch error, got=%#v", rs.Billing)
	}
}

func TestGoFetch_FetchReturnsContextCanceled_NoPublish(t *testing.T) {
	orig := fetchBills
	defer func() { fetchBills = orig }()

	fetchBills = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (m.BillingData, error) {
		return m.BillingData{}, context.Canceled
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	GoFetch(g, ctx, logger, 50*time.Millisecond, cfg, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("group returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if rs.Billing != (m.BillingData{}) {
		t.Errorf("expected no publish on context cancellation, got=%#v", rs.Billing)
	}
}

func TestGoFetch_CancelBeforePublish_NoPublish(t *testing.T) {
	orig := fetchBills
	defer func() { fetchBills = orig }()

	// фетч «успешный», но ответ приходит ПОСЛЕ истечения timeout в GoFetch
	fetchBills = func(ctx context.Context, logger *slog.Logger, cfg *config.CfgApp) (m.BillingData, error) {
		time.Sleep(40 * time.Millisecond) // дольше, чем timeout ниже
		return m.BillingData{
			CreateCustomer: true,
			Purchase:       true,
		}, nil
	}

	logger := testLogger()
	cfg := &config.CfgApp{}
	var rs m.ResultSetT
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	GoFetch(g, ctx, logger, 10*time.Millisecond, cfg, &rs, &mu) // маленький timeout

	if err := g.Wait(); err != nil {
		t.Fatalf("group returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// после таймаута GoFetch делает select{ <-ctx.Done() } и не публикует
	if rs.Billing != (m.BillingData{}) {
		t.Errorf("expected no publish when ctx timed out before publish, got=%#v", rs.Billing)
	}
}
