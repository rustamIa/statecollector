package voicedata

import (
	"context"
	"errors"
	"main/internal/fileutil"
	m "main/internal/model"
	"main/internal/validateStruct"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// -----------------------------------------------------------------------------

func TestGoFetchVoice_Success_PublishesResult(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	const sample = ` BL;58;930;E-Voic;0.65;738;83;52
AT40673Transparentalls;0.62;581;38;10
BG;40;609;E-Voice;0.86;160;36;5
DK;11;743;JustPhone;0.67;82;74;41`

	const wantStr = `BG;40;609;E-Voice;0.86;160;36;5
DK;11;743;JustPhone;0.67;82;74;41`

	fileutil.FileOpener = func(_ string) ([]byte, error) { return []byte(sample), nil }

	// убедимся, что валидаторы подтянулись (как и в fetch_test.go)
	_ = validateStruct.Struct(struct{}{})

	var (
		rs m.ResultSetT
		mu sync.Mutex
		g  errgroup.Group
	)
	ctx := context.Background()

	GoFetch(&g, ctx, testLogger(), 0, makeCfg(), &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected group error: %v", err)
	}

	got := VoiceCallSliceToString(rs.VoiceCall)
	if got != wantStr {
		t.Fatalf("mismatch.\nwant:\n%s\n\ngot:\n%s", wantStr, got)
	}
}

func TestGoFetchVoice_Timeout_BeforePublish(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	// Дадим задержку в «файле», чтобы внутри GoFetchSMS успел сработать timeout контекста,
	// и ветка "cancelled before publish" не записала результат в rs.
	const sample = `RU;86;297;TransparentCalls;0.9;120;80;30`
	fileutil.FileOpener = func(_ string) ([]byte, error) {
		time.Sleep(100 * time.Millisecond) // дольше, чем timeout ниже
		return []byte(sample), nil
	}

	_ = validateStruct.Struct(struct{}{})

	var (
		rs m.ResultSetT
		mu sync.Mutex
		g  errgroup.Group
	)
	ctx := context.Background()

	GoFetch(&g, ctx, testLogger(), 5*time.Millisecond, makeCfg(), &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected group error: %v", err)
	}

	if len(rs.VoiceCall) != 0 {
		t.Fatalf("expected NO publish due to timeout, but got %d rows", len(rs.VoiceCall))
	}
}

func TestGoFetchVoice_FetchError_NoPublish(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	// Симулируем ошибку чтения файла (Fetch вернёт err != nil, не связанную с ctx)
	fileutil.FileOpener = func(_ string) ([]byte, error) {
		return nil, errors.New("boom")
	}

	_ = validateStruct.Struct(struct{}{})

	var (
		rs m.ResultSetT
		mu sync.Mutex
		g  errgroup.Group
	)
	ctx := context.Background()

	GoFetch(&g, ctx, testLogger(), 0, makeCfg(), &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected group error: %v", err)
	}

	if len(rs.VoiceCall) != 0 {
		t.Fatalf("expected NO publish on fetch error, but got %d rows", len(rs.VoiceCall))
	}
}

func TestGoFetchVoice_ParentContextAlreadyCancelled_NoPublish(t *testing.T) {
	origOpen := fileutil.FileOpener
	defer func() { fileutil.FileOpener = origOpen }()

	// Быстрый валидный ответ из "файла"
	const sample = `RU;86;297;TransparentCalls;0.9;120;80;30`
	fileutil.FileOpener = func(_ string) ([]byte, error) { return []byte(sample), nil }

	// Прогреем валидаторы, как в остальных тестах
	_ = validateStruct.Struct(struct{}{})

	var (
		rs m.ResultSetT
		mu sync.Mutex
		g  errgroup.Group
	)

	// Уже отменённый parent context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// timeout = 0, чтобы проверять именно реакцию на отменённый parent
	GoFetch(&g, ctx, testLogger(), 0, makeCfg(), &rs, &mu)

	if err := g.Wait(); err != nil {
		t.Fatalf("unexpected group error: %v", err)
	}

	// Ожидаем, что публикации не было
	if len(rs.VoiceCall) != 0 {
		t.Fatalf("expected NO publish when parent ctx is cancelled, but got %d rows", len(rs.VoiceCall))
	}
}
