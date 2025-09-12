// internal/httpx/fetch.go
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"log/slog"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type DecoderFunc[T any] func(r io.Reader) ([]T, error)

// Общая функция: делает GET, проверяет статус, декодирует массив элементов.
func FetchArray[T any](
	ctx context.Context,
	log *slog.Logger,
	client Doer,
	url string,
	decode DecoderFunc[T],
	op string,
) ([]T, error) {
	l := log.With(slog.String("op", op), slog.String("url", url))
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //Если ctx будет отменён (graceful shutdown наверху), транспорт net/http прервёт операцию: Do или последующее чтение тела вернёт ошибку (типично context canceled).
	if err != nil {
		l.Error("build request", slog.Any("err", err))
		return nil, fmt.Errorf("%s: build request: %w", op, err)
	}

	res, err := client.Do(req)
	if err != nil {
		l.Error("do http-request", slog.Any("err", err))
		return nil, fmt.Errorf("%s: do request: %w", op, err)
	}
	defer func() {
		// гарантируем дренирование, чтобы не терять keep-alive
		// чтобы соединение из пула http.Client могло переиспользоваться и не падала производительность всплесками новых TCP-коннектов.
		_, _ = io.Copy(io.Discard, res.Body) //В net/http (HTTP/1.1) соединение можно вернуть в пул только если тело ответа дочитано до EOF и закрыто; но это на  случай если мы все не вычитали, а "упали"
		_ = res.Body.Close()
	}()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		err = fmt.Errorf("%s: unexpected HTTP status: %s (%d)", op, res.Status, res.StatusCode)
		l.Error("bad status", slog.Any("err", err), slog.Int("status_code", res.StatusCode))
		return nil, err
	}

	items, err := decode(res.Body)
	if err != nil {
		l.Error("decode body", slog.Any("err", err))
		return nil, fmt.Errorf("%s: decode body: %w", op, err)
	}

	l.Info("fetched",
		slog.Duration("dur", time.Since(start)),
		slog.Int("status_code", res.StatusCode),
	)

	return items, nil
}
