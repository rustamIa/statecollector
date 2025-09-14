package incidentdata

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"main/config"
	m "main/internal/model"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func TestBuildSortedIncident_Empty_ReturnsNil(t *testing.T) {
	var in []m.IncidentData
	got := BuildSortedIncident(in)
	if got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestBuildSortedIncident_AllActive_StaysInOrder(t *testing.T) {
	in := []m.IncidentData{
		{Topic: "A", Status: "active"},
		{Topic: "B", Status: "active"},
		{Topic: "C", Status: "active"},
	}
	got := BuildSortedIncident(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("expected same order, got=%v want=%v", got, in)
	}
}

func TestBuildSortedIncident_NoActive_StaysInOrder(t *testing.T) {
	in := []m.IncidentData{
		{Topic: "A", Status: "closed"},
		{Topic: "B", Status: "closed"},
		{Topic: "C", Status: "closed"},
	}
	got := BuildSortedIncident(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("expected same order when no active, got=%v want=%v", got, in)
	}
}

func TestBuildSortedIncident_Mixed_ActiveFirstStable(t *testing.T) {
	in := []m.IncidentData{
		{Topic: "A1", Status: "active"},
		{Topic: "X", Status: "closed"},
		{Topic: "A2", Status: "active"},
		{Topic: "Y", Status: "closed"},
		{Topic: "Z", Status: "closed"},
		{Topic: "A3", Status: "active"},
	}
	want := []m.IncidentData{
		// все active — в исходном порядке
		{Topic: "A1", Status: "active"},
		{Topic: "A2", Status: "active"},
		{Topic: "A3", Status: "active"},
		// затем остальные — алгоритм сохраняет их исходный порядок
		{Topic: "X", Status: "closed"},
		{Topic: "Y", Status: "closed"},
		{Topic: "Z", Status: "closed"},
	}
	got := BuildSortedIncident(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed case: got=%v want=%v", got, want)
	}
}

func TestBuildSortedIncident_TreatsNonActiveAsOther(t *testing.T) {
	// Если в данных встретится неожиданный статус — он должен уйти в «хвост».
	in := []m.IncidentData{
		{Topic: "A", Status: "active"},
		{Topic: "U1", Status: "unknown"},
		{Topic: "B", Status: "active"},
		{Topic: "C", Status: "closed"},
		{Topic: "U2", Status: "unknown"},
	}
	want := []m.IncidentData{
		{Topic: "A", Status: "active"},
		{Topic: "B", Status: "active"},
		{Topic: "U1", Status: "unknown"},
		{Topic: "C", Status: "closed"},
		{Topic: "U2", Status: "unknown"},
	}
	got := BuildSortedIncident(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non-active statuses should be after active: got=%v want=%v", got, want)
	}
}

func TestBuildSortedIncident_DoesNotMutateInput(t *testing.T) {
	in := []m.IncidentData{
		{Topic: "A", Status: "closed"},
		{Topic: "B", Status: "active"},
		{Topic: "C", Status: "closed"},
	}
	// снимок входа
	snapshot := append([]m.IncidentData(nil), in...)

	_ = BuildSortedIncident(in) // результат нам не важен, проверяем неизменность входа

	if !reflect.DeepEqual(in, snapshot) {
		t.Fatalf("input mutated: got=%v want=%v", in, snapshot)
	}
}

// фейковый сервис для инцидентов
type fakeIncidentService struct {
	delay     time.Duration
	result    []m.IncidentData
	err       error
	ignoreCtx bool // если true — игнорируем ctx.Done() и «успешно» завершаемся после delay
}

func (f *fakeIncidentService) Fetch(ctx context.Context) ([]m.IncidentData, error) {
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

// «тихий» логгер
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// ---- tests GoFetch

func TestGoFetch_Incidents_Success_PublishesSortedResult(t *testing.T) {
	// вход умышленно «перемешан» — active должны оказаться сверху в исходном порядке
	in := []m.IncidentData{
		{Topic: "X", Status: "closed"},
		{Topic: "A1", Status: "active"},
		{Topic: "Y", Status: "closed"},
		{Topic: "A2", Status: "active"},
		{Topic: "Z", Status: "closed"},
	}
	fs := &fakeIncidentService{result: in}

	// подмена фабрики
	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportIncidenter { return fs }
	t.Cleanup(func() { newService = prev }) // вернём фабрику обратно

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, ctx, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	// ожидаем, что active идут первыми (A1, A2), затем остальные (X, Y, Z)
	want := []m.IncidentData{
		{Topic: "A1", Status: "active"},
		{Topic: "A2", Status: "active"},
		{Topic: "X", Status: "closed"},
		{Topic: "Y", Status: "closed"},
		{Topic: "Z", Status: "closed"},
	}
	if !reflect.DeepEqual(rs.Incidents, want) {
		t.Fatalf("rs.Incidents = %v, want %v", rs.Incidents, want)
	}
}

func TestGoFetch_Incidents_FetchCancelledByTimeout_DoesNotPublish(t *testing.T) {
	// фейк уважает контекст и «зависает» дольше таймаута
	fs := &fakeIncidentService{delay: 50 * time.Millisecond}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportIncidenter { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	// внутренний timeout меньше, чем задержка
	GoFetch(g, ctx, quietLogger(), 10*time.Millisecond, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Incidents != nil && len(rs.Incidents) != 0 {
		t.Fatalf("expected no publish, got rs.Incidents=%v", rs.Incidents)
	}
}

func TestGoFetch_Incidents_ErrorFromFetch_DoesNotPublish(t *testing.T) {
	fs := &fakeIncidentService{err: errors.New("boom")}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportIncidenter { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, ctx, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Incidents != nil && len(rs.Incidents) != 0 {
		t.Fatalf("expected no publish, got rs.Incidents=%v", rs.Incidents)
	}
}

func TestGoFetch_Incidents_CancelledBeforePublish_DoesNotPublish(t *testing.T) {
	// фейк игнорирует ctx и вернёт успех после задержки, но timeout истечёт раньше —
	// должна сработать ветка "cancelled before publish".
	fs := &fakeIncidentService{
		delay:     30 * time.Millisecond,
		result:    []m.IncidentData{{Topic: "A", Status: "active"}},
		ignoreCtx: true,
	}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportIncidenter { return fs }
	t.Cleanup(func() { newService = prev })

	ctx := context.Background()
	g, _ := errgroup.WithContext(ctx)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, ctx, quietLogger(), 10*time.Millisecond, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Incidents != nil && len(rs.Incidents) != 0 {
		t.Fatalf("expected no publish, got rs.Incidents=%v", rs.Incidents)
	}
}

func TestGoFetch_Incidents_ParentContextCancelled_DoesNotPublish(t *testing.T) {
	// фейк уважает ctx, но мы отменим родительский контекст
	fs := &fakeIncidentService{delay: 50 * time.Millisecond}

	prev := newService
	newService = func(_ *slog.Logger, _ *config.CfgApp, _ *http.Client) supportIncidenter { return fs }
	t.Cleanup(func() { newService = prev })

	parent, cancel := context.WithCancel(context.Background())
	g, _ := errgroup.WithContext(parent)

	var rs m.ResultSetT
	var mu sync.Mutex

	GoFetch(g, parent, quietLogger(), 0, &http.Client{}, &config.CfgApp{}, &rs, &mu)

	// отменяем родительский контекст чуть позже старта
	time.AfterFunc(10*time.Millisecond, cancel)

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() error = %v", err)
	}

	if rs.Incidents != nil && len(rs.Incidents) != 0 {
		t.Fatalf("expected no publish, got rs.Incidents=%v", rs.Incidents)
	}
}
