package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
	h.store.FinishCommand(id, body.Status, body.Output)
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
