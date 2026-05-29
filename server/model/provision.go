package model

import "time"

type VarDef struct {
	Name        string `yaml:"name"        json:"name"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required"    json:"required"`
	Default     string `yaml:"default"     json:"default"`
}

type ConnectorSpec struct {
	Name string            `json:"name"`
	Vars map[string]string `json:"vars"`
}

type Provision struct {
	Token      string          `json:"token"`
	Name       string          `json:"name"`
	HostType   string          `json:"host_type"`
	Connectors []ConnectorSpec `json:"connectors"`
	CreatedAt  time.Time       `json:"created_at"`
	ClaimedBy  string          `json:"claimed_by,omitempty"`
}

// ConnectorMeta is returned by GET /api/v1/connectors.
type ConnectorMeta struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Category    string   `json:"category"`
	Vars        []VarDef `json:"vars"`
}

// AgentConfig is returned to the agent when it bootstraps with a token.
type AgentConfig struct {
	Hostname   string          `json:"hostname"`
	Connectors []ConnectorFile `json:"connectors"`
}

type ConnectorFile struct {
	Name    string `json:"name"`
	Content string `json:"content"` // raw YAML
}
