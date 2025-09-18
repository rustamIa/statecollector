package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

// ---------- утилиты для тестов

// логгер, который пишет в никуда (чтобы slog не паниковал на nil)
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

// ReadCloser, который считает, сколько байт прочитали, и отмечает Close()
type countingBody struct {
	r      io.Reader
	read   int
	closed bool
}

func newCountingBody(s string) *countingBody {
	return &countingBody{r: bytes.NewBufferString(s)}
}
func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.read += n
	return n, err
}
func (b *countingBody) Close() error {
	b.closed = true
	return nil
}

// простой JSON-декодер в []T
func decodeJSON[T any](r io.Reader) ([]T, error) {
	var v []T
	err := json.NewDecoder(r).Decode(&v)
	return v, err
}

// Doer, который возвращает заранее подготовленный ответ/ошибку
type fakeClient struct {
	resp *http.Response
	err  error
}

func (c *fakeClient) Do(_ *http.Request) (*http.Response, error) {
	return c.resp, c.err
}

// Doer, который ждёт отмены req.Context() и возвращает ctx.Err()
type cancelAwareClient struct{}

func (c *cancelAwareClient) Do(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

// ---------- тесты

type item struct {
	A int `json:"a"`
}

func TestFetchArray_Success(t *testing.T) {
	logger := discardLogger()
	body := newCountingBody(`[{"a":1},{"a":2}]`)
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}
	client := &fakeClient{resp: resp}

	ctx := context.Background()
	got, err := FetchArray[item](ctx, logger, client, "http://example", decodeJSON[item], "test-op")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].A != 1 || got[1].A != 2 {
		t.Fatalf("bad decode result: %#v", got)
	}
	// при успехе декодер дочитает до EOF; defer всё равно вызовет Close()
	if !body.closed {
		t.Fatalf("response body not closed")
	}
	// можно не проверять exact drain, но тут он равен длине строки
	wantRead := len(`[{"a":1},{"a":2}]`)
	if body.read != wantRead {
		t.Fatalf("body not fully read: read=%d want=%d", body.read, wantRead)
	}
}

func TestFetchArray_Non2xxStatus_DrainsAndCloses(t *testing.T) {
	logger := discardLogger()
	body := newCountingBody(`{"err":"boom"}`)
	resp := &http.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Body:       body,
	}
	client := &fakeClient{resp: resp}

	_, err := FetchArray[item](context.Background(), logger, client, "http://example", decodeJSON[item], "op")
	if err == nil {
		t.Fatalf("expected error on non-2xx")
	}
	if !body.closed {
		t.Fatalf("response body not closed on error")
	}
	if body.read != len(`{"err":"boom"}`) {
		t.Fatalf("body not fully drained on error: read=%d", body.read)
	}
}

func TestFetchArray_DecodeError_DrainsAndCloses(t *testing.T) {
	logger := discardLogger()
	body := newCountingBody(`{not-json-array}`)
	resp := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       body,
	}
	client := &fakeClient{resp: resp}

	_, err := FetchArray[item](context.Background(), logger, client, "http://example", decodeJSON[item], "op")
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if !body.closed {
		t.Fatalf("response body not closed on decode error")
	}
	// декодер мог прочитать часть; defer обязан дочитать остаток
	if body.read != len(`{not-json-array}`) {
		t.Fatalf("body not fully drained after decode error: read=%d", body.read)
	}
}

func TestFetchArray_ClientDoError(t *testing.T) {
	logger := discardLogger()
	client := &fakeClient{err: errors.New("network down")}
	_, err := FetchArray[item](context.Background(), logger, client, "http://example", decodeJSON[item], "op")
	if err == nil {
		t.Fatalf("expected client.Do error")
	}
}

func TestFetchArray_ContextCanceled(t *testing.T) {
	logger := discardLogger()
	client := &cancelAwareClient{}

	ctx, cancel := context.WithCancel(context.Background())
	// отменяем почти сразу — Do увидит Done
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := FetchArray[item](ctx, logger, client, "http://example", decodeJSON[item], "op")
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
}
