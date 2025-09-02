package incidentdata

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

type IncidentData struct {
	Topic  string `json:"topic" validate:"required"`
	Status string `json:"status" validate:"oneof=active closed"`
}

// В продакшене передавайте http.Client извне, чтобы реиспользовать пул соединений.
type Service struct {
	log    *slog.Logger
	cfg    *config.CfgApp
	client *http.Client
}

func (v IncidentData) Validate() error {
	return validateStruct.Struct(v)
}

func NewService(log *slog.Logger, cfg *config.CfgApp, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Service{log: log, cfg: cfg, client: client}
}

func (s *Service) Fetch(ctx context.Context) ([]IncidentData, error) {
	decode := func(r io.Reader) ([]IncidentData, error) {
		return jsonx.DecodeArrayFromReader[IncidentData](r, &jsonx.Options[IncidentData]{})
	}

	return httpx.FetchArray[IncidentData](
		ctx,
		s.log,
		s.client,
		s.cfg.PathIncidentData,
		decode,
		"incidentdata.Fetch",
	)
}
