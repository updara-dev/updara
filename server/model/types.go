package model

import "time"

type Host struct {
	ID           string    `json:"id"`
	Hostname     string    `json:"hostname"`
	AgentVersion string    `json:"agent_version"`
	LastSeen     time.Time `json:"last_seen"`
}

type CheckResult struct {
	Connector       string            `json:"connector"`
	DisplayName     string            `json:"display_name"`
	Category        string            `json:"category"`
	Values          map[string]string `json:"values"`
	UpdateAvailable bool              `json:"update_available"`
	Changelog       string            `json:"changelog"`
	Error           string            `json:"error,omitempty"`
	CheckedAt       time.Time         `json:"checked_at"`
}

type ReportRequest struct {
	Hostname     string        `json:"hostname"`
	AgentVersion string        `json:"agent_version"`
	Token        string        `json:"token,omitempty"`
	Results      []CheckResult `json:"results"`
}

type HostStatus struct {
	Host    Host          `json:"host"`
	Results []CheckResult `json:"results"`
}
