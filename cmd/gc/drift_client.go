package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/api/genclient"
)

// httpSupervisorClient is the production SupervisorClient implementation.
// It calls the supervisor's /health endpoint over plain HTTP to discover
// build identity and to verify post-restart readiness.
//
// /health is the right endpoint for both purposes: it is the supervisor's
// canonical liveness probe (no auth, no per-city scope, fast), and it
// reports build_id which is the load-bearing field for drift detection.
type httpSupervisorClient struct {
	baseURL    string
	httpClient *http.Client
}

// newHTTPSupervisorClient returns a client targeting baseURL. The base
// URL must include scheme and authority (e.g. "http://127.0.0.1:8080");
// /health is appended at request time.
func newHTTPSupervisorClient(baseURL string) *httpSupervisorClient {
	return &httpSupervisorClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Status fetches /health and projects the typed response onto SupervisorStatus.
func (c *httpSupervisorClient) Status(ctx context.Context) (SupervisorStatus, error) {
	client, err := genclient.NewClientWithResponses(c.baseURL, genclient.WithHTTPClient(c.httpClient))
	if err != nil {
		return SupervisorStatus{}, err
	}
	resp, err := client.GetHealthWithResponse(ctx)
	if err != nil {
		return SupervisorStatus{}, err
	}
	if resp.StatusCode()/100 != 2 {
		return SupervisorStatus{}, fmt.Errorf("supervisor /health returned %d: %s", resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
	if resp.JSON200 == nil {
		return SupervisorStatus{}, fmt.Errorf("supervisor /health returned %d with empty JSON body", resp.StatusCode())
	}
	return supervisorStatusFromHealth(resp.JSON200), nil
}

func supervisorStatusFromHealth(body *genclient.SupervisorHealthOutputBody) SupervisorStatus {
	var buildID string
	if body.BuildId != nil {
		buildID = *body.BuildId
	}
	status := SupervisorStatus{BuildID: buildID, UptimeSec: int(body.UptimeSec)}
	if body.PackRoots != nil {
		status.PackRoots = make([]PackRootStatus, 0, len(*body.PackRoots))
		for _, root := range *body.PackRoots {
			status.PackRoots = append(status.PackRoots, PackRootStatus{
				Dir:      root.Dir,
				ParsedAt: root.ParsedAt,
			})
		}
	}
	return status
}

// Ping issues a GET /health and returns nil iff the response is 2xx.
// Used by PollReady after RestartSupervisor to wait for the new
// supervisor process to come up.
func (c *httpSupervisorClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("supervisor /health returned %d", resp.StatusCode)
	}
	return nil
}
