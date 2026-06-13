package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/updara/server/model"
	"gopkg.in/yaml.v3"
)

// POST /api/v1/hosts/{hostname}/update/{connector}
func (h *Handler) triggerUpdate(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	connectorName := r.PathValue("connector")

	cmd := h.connectorUpdateCmd(connectorName)
	if cmd == "" {
		http.Error(w, "connector has no update command", http.StatusBadRequest)
		return
	}

	b := make([]byte, 8)
	rand.Read(b)
	c := model.Command{
		ID:        hex.EncodeToString(b),
		HostID:    hostname,
		Connector: connectorName,
		Cmd:       cmd,
		Status:    model.CmdStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	h.store.AddCommand(c)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

// GET /api/v1/hosts/{hostname}/commands/pending  (called by agent)
func (h *Handler) pendingCommands(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	cmds := h.store.ClaimPending(hostname)
	if cmds == nil {
		cmds = []model.Command{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cmds)
}

// POST /api/v1/commands/{id}/result  (called by agent)
func (h *Handler) commandResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
		Output string `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cmd, hasCmd := h.store.GetCommand(id)
	h.store.FinishCommand(id, body.Status, body.Output)

	// After a successful connector update, queue a recheck so the result refreshes immediately
	// instead of waiting up to an hour for the next scheduled check.
	if hasCmd && body.Status == model.CmdStatusDone && !strings.HasPrefix(cmd.Connector, "__") {
		b := make([]byte, 8)
		rand.Read(b)
		h.store.AddCommand(model.Command{
			ID:        hex.EncodeToString(b),
			HostID:    cmd.HostID,
			Connector: "__recheck__",
			Cmd:       "nohup sh -c 'sleep 3 && systemctl restart updara-agent' >/dev/null 2>&1 &",
			Status:    model.CmdStatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/hosts/{hostname}/commands  (polled by dashboard)
func (h *Handler) hostCommands(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	cmds := h.store.HostCommands(hostname)
	if cmds == nil {
		cmds = []model.Command{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cmds)
}

// POST /api/v1/hosts/{hostname}/sync
func (h *Handler) syncAgent(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")

	b := make([]byte, 8)
	rand.Read(b)
	c := model.Command{
		ID:        hex.EncodeToString(b),
		HostID:    hostname,
		Connector: "__sync__",
		Cmd:       "nohup sh -c 'sleep 2 && systemctl restart updara-agent' >/dev/null 2>&1 &",
		Status:    model.CmdStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	h.store.AddCommand(c)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

// POST /api/v1/hosts/{hostname}/connectors/{connector}/install
func (h *Handler) installConnector(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	connector := r.PathValue("connector")

	var req struct {
		Vars map[string]string `json:"vars"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Re-enable if previously removed
	h.store.EnableConnector(hostname, connector)

	// Update provision: add connector + vars so the agent syncs it going forward
	svcCmd := ""
	if p, ok := h.store.ProvisionByHost(hostname); ok {
		found := false
		for i, c := range p.Connectors {
			if c.Name == connector {
				if req.Vars != nil {
					if p.Connectors[i].Vars == nil {
						p.Connectors[i].Vars = map[string]string{}
					}
					for k, v := range req.Vars {
						p.Connectors[i].Vars[k] = v
					}
				}
				found = true
				break
			}
		}
		if !found {
			p.Connectors = append(p.Connectors, model.ConnectorSpec{Name: connector, Vars: req.Vars})
		}
		p.ServerURL = h.serverURL(r)
		h.store.AddProvision(*p)

		if len(req.Vars) > 0 {
			svc := h.buildServiceFile(p.ServerURL, p.Token, p.Name, p.Connectors)
			svcB64 := base64.StdEncoding.EncodeToString([]byte(svc))
			svcCmd = fmt.Sprintf(
				" && printf '%%s' '%s' | base64 -d > /etc/systemd/system/updara-agent.service && systemctl daemon-reload",
				svcB64,
			)
		}
	}

	cmd := "mkdir -p /etc/updara/connectors && curl -sf " +
		h.publicURL + "/api/v1/connectors/" + connector + "/yaml" +
		" -o /etc/updara/connectors/" + connector + ".yaml" +
		svcCmd +
		" && nohup sh -c 'sleep 2 && systemctl restart updara-agent' >/dev/null 2>&1 &"

	b := make([]byte, 8)
	rand.Read(b)
	c := model.Command{
		ID:        hex.EncodeToString(b),
		HostID:    hostname,
		Connector: "__install__" + connector,
		Cmd:       cmd,
		Status:    model.CmdStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	h.store.AddCommand(c)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

func (h *Handler) connectorUpdateCmd(name string) string {
	data, err := os.ReadFile(filepath.Join(h.connectorsDir, name+".yaml"))
	if err != nil {
		return ""
	}
	var c struct {
		Update struct {
			Command string `yaml:"command"`
		} `yaml:"update"`
	}
	yaml.Unmarshal(data, &c)
	return c.Update.Command
}
