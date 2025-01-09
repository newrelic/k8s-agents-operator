package instrumentation

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/yaml.v3"
)

type Health struct {
	Healthy            bool               `json:"healthy" yaml:"healthy"`
	Status             string             `json:"status" yaml:"status"`
	StartTime          int64              `json:"start_time_unix_nano" yaml:"start_time_unix_nano"`
	StatusTime         int64              `json:"status_time_unix_nano" yaml:"status_time_unix_nano"`
	LastError          string             `json:"last_error" yaml:"last_error"`
	ComponentHealthMap map[string]*Health `json:"component_health_map,omitempty" yaml:"component_health_map,omitempty"`
}

type HealthCheck interface {
	GetHealth(ctx context.Context, url string) (health Health, err error)
}

type HealthCheckApi struct {
	httpClient *http.Client
}

func NewHealthCheckApi(httpClient *http.Client) *HealthCheckApi {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &HealthCheckApi{
		httpClient: httpClient,
	}
}

func (h *HealthCheckApi) GetHealth(ctx context.Context, url string) (health Health, err error) {
	var (
		httpReq *http.Request
		res     *http.Response
		body    []byte
	)
	if httpReq, err = http.NewRequest(http.MethodGet, url, nil); err != nil {
		return health, fmt.Errorf("failed to create request > %w", err)
	}
	if res, err = h.httpClient.Do(httpReq.WithContext(ctx)); err != nil {
		return health, fmt.Errorf("failed to send request > %w", err)
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return health, fmt.Errorf("failed to get expected response code of 200, got %d > %w", res.StatusCode, err)
	}
	if body, err = io.ReadAll(res.Body); err != nil {
		return health, fmt.Errorf("failed to read response body > %w", err)
	}
	if err = yaml.Unmarshal(body, &health); err != nil {
		return health, fmt.Errorf("failed to parse response > %w", err)
	}
	return health, nil
}
