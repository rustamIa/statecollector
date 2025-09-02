package mmsdata

import (
	"context"
	"io"
	"net/http"
	"time"

	"log/slog"
	"main/config"
	"main/internal/httpx"
	"main/internal/jsonx"
	"main/internal/validateStruct"
)

// структура для MMSData
type MMSData struct {
	Country      string `json:"country" validate:"iso3166_1_alpha2"`
	Provider     string `json:"provider" validate:"oneof=Topolo Rond Kildy"`
	Bandwidth    string `json:"bandwidth" validate:"required,num0to100"`
	ResponseTime string `json:"response_time" validate:"required,number"`
}

func (v MMSData) Validate() error {
	return validateStruct.Struct(v)
}

// В продакшене передавайте http.Client извне, чтобы реиспользовать пул соединений.
type Service struct {
	log    *slog.Logger
	cfg    *config.CfgApp
	client *http.Client
}

func NewService(log *slog.Logger, cfg *config.CfgApp, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Service{log: log, cfg: cfg, client: client}
}

func (s *Service) Fetch(ctx context.Context) ([]MMSData, error) {
	decode := func(r io.Reader) ([]MMSData, error) {
		return jsonx.DecodeArrayFromReader[MMSData](r, &jsonx.Options[MMSData]{})
	}

	return httpx.FetchArray[MMSData](
		ctx,
		s.log,
		s.client,
		s.cfg.PathMmsData,
		decode,
		"mmsdata.Fetch",
	)
}

/*
import (
	"context"
	"fmt"
	"io"
	"net/http"

	"log/slog"
	"main/config"
	"main/internal/jsonx"
	"main/internal/validateStruct"
	"main/sl"
	"time"
)

// структура для MMSData
type MMSData struct {
	Country      string `json:"country" validate:"iso3166_1_alpha2"`
	Provider     string `json:"provider" validate:"oneof=Topolo Rond Kildy"`
	Bandwidth    string `json:"bandwidth" validate:"required,num0to100"`
	ResponseTime string `json:"response_time" validate:"required,number"`
}

// В продакшене передавайте http.Client извне, чтобы реиспользовать пул соединений.
type Service struct {
	log    *slog.Logger
	cfg    *config.CfgApp
	client *http.Client
}

func (v MMSData) Validate() error {
	return validateStruct.Struct(v)
}

func NewService(log *slog.Logger, cfg *config.CfgApp, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Service{log: log, cfg: cfg, client: client}
}

func (s *Service) Fetch(ctx context.Context) ([]MMSData, error) {
	const op = "mmsdata.Fetch"
	log := s.log.With(
		slog.String("op", op),
		slog.String("url", s.cfg.PathMmsData),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.PathMmsData, nil)
	if err != nil {
		log.Error("build request", sl.Err(err))
		return nil, fmt.Errorf("%s: build request: %w", op, err)
	}

	res, err := s.client.Do(req)
	if err != nil {
		log.Error("do http-request", sl.Err(err))
		return nil, fmt.Errorf("%s: do request: %w", op, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, res.Body) // дочитать тело
		err = fmt.Errorf("%s: unexpected HTTP status: %s (%d)", op, res.Status, res.StatusCode)
		log.Error("bad status", sl.Err(err), slog.Int("status_code", res.StatusCode))
		return nil, err
	}

	// Вариант А: читаем всё в память
	// body, err := io.ReadAll(res.Body)
	// if err != nil {
	// 	log.Error("read body", sl.Err(err))
	// 	return nil, fmt.Errorf("%s: read body: %w", op, err)
	// }
	//data, err := jsonx.DecodeArray[SupportData](body, &jsonx.Options[SupportData]{})

	// Вариант Б: стриминговый (без ReadAll)
	data, err := jsonx.DecodeArrayFromReader[MMSData](res.Body, &jsonx.Options[MMSData]{})
	if err != nil {
		log.Error("Error by decode json in body from http-Get response", sl.Err(err))
		return nil, fmt.Errorf("%s: failed by decode&validate json: %w", op, err)
	}
	return data, nil

}
*/
