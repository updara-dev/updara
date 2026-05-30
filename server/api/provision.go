package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/updara/server/model"
	"gopkg.in/yaml.v3"
)

// ── Create provision ─────────────────────────────────────────────────────────

func (h *Handler) createProvision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string                 `json:"name"`
		HostType   string                 `json:"host_type"`
		Connectors []model.ConnectorSpec  `json:"connectors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	token := generateToken()
	h.store.AddProvision(model.Provision{
		Token:      token,
		Name:       req.Name,
		HostType:   req.HostType,
		Connectors: req.Connectors,
		CreatedAt:  time.Now(),
	})

	serverURL := h.serverURL(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":       token,
		"install_cmd": fmt.Sprintf("curl -fsSL '%s/install?token=%s' | sh", serverURL, token),
	})
}

// ── List provisions ───────────────────────────────────────────────────────────

func (h *Handler) listProvisions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.store.AllProvisions())
}

func (h *Handler) deleteProvision(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	h.store.DeleteProvision(token)
	w.WriteHeader(http.StatusNoContent)
}

// ── Agent bootstrap ───────────────────────────────────────────────────────────

func (h *Handler) getProvisionConfig(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	p, ok := h.store.GetProvision(token)
	if !ok {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}

	var files []model.ConnectorFile
	for _, spec := range p.Connectors {
		content, err := os.ReadFile(filepath.Join(h.connectorsDir, spec.Name+".yaml"))
		if err != nil {
			continue
		}
		files = append(files, model.ConnectorFile{
			Name:    spec.Name,
			Content: string(content),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.AgentConfig{
		Hostname:   p.Name,
		Connectors: files,
	})
}

// ── List available connectors ─────────────────────────────────────────────────

func (h *Handler) listConnectors(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(h.connectorsDir)

	type connMeta struct {
		Name        string          `yaml:"name"         json:"name"`
		DisplayName string          `yaml:"display_name" json:"display_name"`
		Category    string          `yaml:"category"     json:"category"`
		Docs        string          `yaml:"docs"         json:"docs"`
		Vars        []model.VarDef  `yaml:"vars"         json:"vars"`
	}

	var out []connMeta
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.connectorsDir, e.Name()))
		if err != nil {
			continue
		}
		var m connMeta
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.Vars == nil {
			m.Vars = []model.VarDef{}
		}
		out = append(out, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ── Install script ────────────────────────────────────────────────────────────

func (h *Handler) serveInstallScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	p, ok := h.store.GetProvision(token)
	if !ok {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}

	serverURL := h.serverURL(r)

	// Build Environment= lines for systemd
	var envLines strings.Builder
	envLines.WriteString(fmt.Sprintf("Environment=SERVER_URL=%s\n", serverURL))
	envLines.WriteString(fmt.Sprintf("Environment=UPDARA_TOKEN=%s\n", token))
	envLines.WriteString(fmt.Sprintf("Environment=HOSTNAME_OVERRIDE=%s\n", p.Name))
	envLines.WriteString("Environment=CONNECTORS_DIR=/etc/updara/connectors\n")
	for _, c := range p.Connectors {
		for k, v := range c.Vars {
			envLines.WriteString(fmt.Sprintf("Environment=%s=%s\n", k, v))
		}
	}

	svc := fmt.Sprintf(`[Unit]
Description=Updara Agent
After=network-online.target
Wants=network-online.target

[Service]
%s
ExecStart=/usr/local/bin/updara-agent
Restart=always
RestartSec=30

[Install]
WantedBy=multi-user.target
`, envLines.String())

	svcB64 := base64.StdEncoding.EncodeToString([]byte(svc))

	script := fmt.Sprintf(`#!/bin/sh
set -e

SERVER_URL="%s"
UPDARA_TOKEN="%s"

echo "[Updara] Installing agent for host: %s"

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  GOARCH=amd64 ;;
  aarch64) GOARCH=arm64 ;;
  armv7l)  GOARCH=arm ;;
  *) echo "[Updara] Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "[Updara] Downloading agent ($GOARCH)..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$SERVER_URL/api/v1/agent/binary/$GOARCH" -o /usr/local/bin/updara-agent
elif command -v wget >/dev/null 2>&1; then
  wget -qO /usr/local/bin/updara-agent "$SERVER_URL/api/v1/agent/binary/$GOARCH"
else
  echo "[Updara] Error: curl or wget is required"; exit 1
fi
chmod +x /usr/local/bin/updara-agent

mkdir -p /etc/updara/connectors

echo "[Updara] Writing systemd service..."
printf '%%s' '%s' | base64 -d > /etc/systemd/system/updara-agent.service

systemctl daemon-reload
systemctl enable --now updara-agent

echo "[Updara] Done! Check: systemctl status updara-agent"
`, serverURL, token, p.Name, svcB64)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, script)
}

// ── Serve agent binary ────────────────────────────────────────────────────────

func (h *Handler) serveAgentBinary(w http.ResponseWriter, r *http.Request) {
	arch := r.PathValue("arch")
	switch arch {
	case "amd64", "arm64", "arm":
	default:
		http.Error(w, "unsupported arch", http.StatusBadRequest)
		return
	}
	path := filepath.Join(h.binariesDir, "updara-agent-"+arch)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "binary not available for "+arch, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="updara-agent"`)
	http.ServeFile(w, r, path)
}

// ── Connector YAML management ─────────────────────────────────────────────────

func (h *Handler) getConnectorYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isValidConnectorName(name) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(h.connectorsDir, name+".yaml"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (h *Handler) saveConnector(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isValidConnectorName(name) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var check any
	if err := yaml.Unmarshal(body, &check); err != nil {
		http.Error(w, "invalid YAML: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(filepath.Join(h.connectorsDir, name+".yaml"), body, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteConnector(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isValidConnectorName(name) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := os.Remove(filepath.Join(h.connectorsDir, name+".yaml")); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidConnectorName(name string) bool {
	return name != "" &&
		!strings.ContainsAny(name, "/\\.") &&
		!strings.HasPrefix(name, "_")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) serverURL(r *http.Request) string {
	if h.publicURL != "" {
		return h.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
