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
	m "main/internal/model"
)

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

func (s *Service) Fetch(ctx context.Context) ([]m.IncidentData, error) {
	decode := func(r io.Reader) ([]m.IncidentData, error) {
		return jsonx.DecodeArrayFromReader[m.IncidentData](r, &jsonx.Options[m.IncidentData]{})
	}

	return httpx.FetchArray[m.IncidentData](
		ctx,
		s.log,
		s.client,
		s.cfg.PathIncidentData,
		decode,
		"incidentdata.Fetch",
	)
}
