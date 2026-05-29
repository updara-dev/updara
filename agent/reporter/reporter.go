package reporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/updara/agent/runner"
)

type Reporter struct {
	serverURL    string
	hostname     string
	agentVersion string
	token        string
	client       *http.Client
}

func New(serverURL, hostname, agentVersion, token string) *Reporter {
	return &Reporter{
		serverURL:    serverURL,
		hostname:     hostname,
		agentVersion: agentVersion,
		token:        token,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

type payload struct {
	Hostname     string          `json:"hostname"`
	AgentVersion string          `json:"agent_version"`
	Token        string          `json:"token,omitempty"`
	Results      []runner.Result `json:"results"`
}

func (r *Reporter) Send(results []runner.Result) error {
	body, err := json.Marshal(payload{
		Hostname:     r.hostname,
		AgentVersion: r.agentVersion,
		Token:        r.token,
		Results:      results,
	})
	if err != nil {
		return err
	}

	resp, err := r.client.Post(
		r.serverURL+"/api/v1/report",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
