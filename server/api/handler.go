package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/updara/server/model"
	"github.com/updara/server/store"
)

type Handler struct {
	store         *store.Store
	connectorsDir string
	binariesDir   string
	publicURL     string
}

func NewHandler(s *store.Store, connectorsDir, binariesDir, publicURL string) *Handler {
	return &Handler{
		store:         s,
		connectorsDir: connectorsDir,
		binariesDir:   binariesDir,
		publicURL:     publicURL,
	}
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	// Agent reports
	mux.HandleFunc("POST /api/v1/report", h.report)

	// Dashboard data
	mux.HandleFunc("GET /api/v1/hosts", h.hosts)
	mux.HandleFunc("GET /api/v1/connectors", h.listConnectors)

	// Update commands
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/update/{connector}", h.triggerUpdate)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/commands/pending", h.pendingCommands)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/commands", h.hostCommands)
	mux.HandleFunc("POST /api/v1/commands/{id}/result", h.commandResult)

	// Host provisioning
	mux.HandleFunc("POST /api/v1/provisions", h.createProvision)
	mux.HandleFunc("GET /api/v1/provisions", h.listProvisions)
	mux.HandleFunc("DELETE /api/v1/provisions/{token}", h.deleteProvision)
	mux.HandleFunc("GET /api/v1/provisions/{token}/config", h.getProvisionConfig)

	// Connector YAML management
	mux.HandleFunc("GET /api/v1/connectors/{name}/yaml", h.getConnectorYAML)
	mux.HandleFunc("PUT /api/v1/connectors/{name}", h.saveConnector)
	mux.HandleFunc("DELETE /api/v1/connectors/{name}", h.deleteConnector)

	// Agent install
	mux.HandleFunc("GET /install", h.serveInstallScript)
	mux.HandleFunc("GET /api/v1/agent/binary/{arch}", h.serveAgentBinary)

	mux.HandleFunc("GET /healthz", h.healthz)

	return cors(mux)
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	var req model.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If agent reports with a token, claim the provision
	if req.Token != "" {
		h.store.ClaimProvision(req.Token, req.Hostname)
	}

	h.store.Upsert(req)
	log.Printf("report from %s: %d result(s)", req.Hostname, len(req.Results))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) hosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.store.AllHosts())
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
