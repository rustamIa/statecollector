package support

import (
	"context"
	"io"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"math"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func expectedWait(total int) int {
	return int(math.Ceil(float64(total) * (60.0 / 18.0)))
}

func TestBuildSortedSupport(t *testing.T) {
	tests := []struct {
		name     string
		input    []m.SupportData
		wantLoad int
		wantWait int
	}{
		{
			name:     "empty slice => no tickets, low load",
			input:    []m.SupportData{},
			wantLoad: 1,
			wantWait: expectedWait(0),
		},
		{
			name: "sum < 9 => low load",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: 3},
				{Topic: "B", ActiveTickets: 5},
			},
			wantLoad: 1,
			wantWait: expectedWait(8), // ceil(8 * 60/18) = 27
		},
		{
			name: "sum = 9 => medium load lower bound",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: 4},
				{Topic: "B", ActiveTickets: 5},
			},
			wantLoad: 2,
			wantWait: expectedWait(9), // 30
		},
		{
			name: "sum = 16 => medium load upper bound",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: 7},
				{Topic: "B", ActiveTickets: 9},
			},
			wantLoad: 2,
			wantWait: expectedWait(16), // 54
		},
		{
			name: "sum > 16 => high load",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: 10},
				{Topic: "B", ActiveTickets: 7},
				{Topic: "C", ActiveTickets: 1},
			},
			wantLoad: 3,
			wantWait: expectedWait(18), // 60
		},
		{
			name: "negatives and zeros are ignored",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: -1},
				{Topic: "B", ActiveTickets: 0},
				{Topic: "C", ActiveTickets: 5},
			},
			wantLoad: 1,
			wantWait: expectedWait(5), // 17
		},
		{
			name: "big number",
			input: []m.SupportData{
				{Topic: "A", ActiveTickets: 100},
			},
			wantLoad: 3,
			wantWait: expectedWait(100), // 334
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := BuildSortedSupport(tc.input)
			want := []int{tc.wantLoad, tc.wantWait}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("BuildSortedSupport() = %v, want %v", got, want)
			}
		})
	}
}

/*
Success_PublishesResult — успешный Fetch публикует в rs.Support [уровень, ожидание], рассчитанный из данных.

FetchCancelledByTimeout — внутренний таймаут срабатывает, Fetch возвращает ctx.Err(), публикации нет.

ErrorFromFetch — любая ошибка (не отмена) не «роняет» группу, но публикации нет.

CancelledBeforePublish — имитируем ситуацию, когда Fetch завершился успешно, но контекст уже истёк; проверяем, что ветка «cancelled before publish» сработала и публикации нет.

ParentContextCancelled — отмена родительского контекста тоже не приводит к публикации.
*/
type fakeService struct {
	delay     time.Duration
	result    []m.SupportData
	err       error
	ignoreCtx bool // если true — игнорируем ctx.Done() и возвращаем успех после задержки
}

func (f *fakeService) Fetch(ctx context.Context) ([]m.SupportData, error) {
	if f.delay > 0 {
		if f.ignoreCtx {
			time.Sleep(f.delay)
		} else {
			select {
			case <-time.After(f.delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return f.result, f.err
}

// создаёт «тихий» логгер
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// ---- tests ----

func TestGoFetch_Success_PublishesResult(t *testing.T) {
	// подготовка фейка: суммарно 10 тикетов => load=2 (9..16), wait = ceil(10*60/18)=34
	fs := &fakeService{
		result: []m.SupportData{
			{Topic: "A", ActiveTickets: 3},
			{Topic: "B", ActiveTickets: 7},
		},
	}

	// подмена фабрики
	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportFetcher { return fs }
	t.Cleanup(func() { newService = prev }) //этакий defer при тестах

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, ctx, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	want := []int{2, expectedWait(10)}
	if !reflect.DeepEqual(rs.Support, want) {
		t.Fatalf("rs.Support = %v, want %v", rs.Support, want)
	}
}

func TestGoFetch_FetchCancelledByTimeout_DoesNotPublish(t *testing.T) {
	// фейк уважает контекст и «зависает» дольше таймаута
	fs := &fakeService{delay: 50 * time.Millisecond}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportFetcher { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	// внутренний timeout маленький
	GoFetch(g, ctx, quietLogger(), 10*time.Millisecond, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	// ожиданий быть не должно — публикации не было
	if rs.Support != nil && len(rs.Support) != 0 {
		t.Fatalf("expected no publish, got rs.Support=%v", rs.Support)
	}
}

func TestGoFetch_ErrorFromFetch_DoesNotPublish(t *testing.T) {
	fs := &fakeService{err: context.DeadlineExceeded} // любая не-nil ошибка (не паника)

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportFetcher { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, ctx, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Support != nil && len(rs.Support) != 0 {
		t.Fatalf("expected no publish, got rs.Support=%v", rs.Support)
	}
}

func TestGoFetch_CancelledBeforePublish_DoesNotPublish(t *testing.T) {
	// фейк ИГНОРИРУЕТ ctx и вернёт успех, но только после задержки.
	// Таймаут истечёт раньше, и в GoFetch сработает ветка "cancelled before publish".
	fs := &fakeService{
		delay:     30 * time.Millisecond,
		result:    []m.SupportData{{Topic: "X", ActiveTickets: 9}},
		ignoreCtx: true,
	}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportFetcher { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	// внутренний timeout меньше, чем delay фейка
	GoFetch(g, ctx, quietLogger(), 10*time.Millisecond, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	// публикации быть не должно (отменили "перед publish")
	if rs.Support != nil && len(rs.Support) != 0 {
		t.Fatalf("expected no publish, got rs.Support=%v", rs.Support)
	}
}

func TestGoFetch_ParentContextCancelled_DoesNotPublish(t *testing.T) {
	// фейк уважает ctx, но мы отменим родительский контекст
	fs := &fakeService{delay: 50 * time.Millisecond}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportFetcher { return fs }
	t.Cleanup(func() { newService = prev })

	parent, cancel := context.WithCancel(context.Background())
	g, _ := errgroup.WithContext(parent)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, parent, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	// Отменяем родительский контекст чуть позже старта
	time.AfterFunc(10*time.Millisecond, cancel)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Support != nil && len(rs.Support) != 0 {
		t.Fatalf("expected no publish, got rs.Support=%v", rs.Support)
	}
}
