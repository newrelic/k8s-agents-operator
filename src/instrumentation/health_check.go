package instrumentation

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/yaml.v3"
)

type Health struct {
	Healthy            bool               `json:"healthy"`
	Status             string             `json:"status"`
	StartTime          string             `json:"start_time_unix_nano"`
	StatusTime         string             `json:"status_time_unix_nano"`
	AgentRunID         string             `json:"agent_run_id"`
	LastError          string             `json:"last_error"`
	ComponentHealthMap map[string]*Health `json:"component_health_map"`
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
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return health, fmt.Errorf("failed to create request > %w", err)
	}

	httpClient := &http.Client{}
	res, err := httpClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		return health, fmt.Errorf("failed to send request > %w", err)
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return health, fmt.Errorf("failed to get expected response code of 200 > %w", err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return health, fmt.Errorf("failed to read response body > %w", err)
	}

	err = yaml.Unmarshal(body, &health)
	if err != nil {
		return health, fmt.Errorf("failed to parse response > %w", err)
	}
	return health, nil
}
