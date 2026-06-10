package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/updara/server/model"
	"github.com/updara/server/notify"
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
	mux.HandleFunc("GET /api/v1/hosts/{hostname}", h.hostDetail)
	mux.HandleFunc("GET /api/v1/connectors", h.listConnectors)

	// Update commands
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/update/{connector}", h.triggerUpdate)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/commands/pending", h.pendingCommands)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/commands", h.hostCommands)
	mux.HandleFunc("POST /api/v1/commands/{id}/result", h.commandResult)

	// Agent sync (restart via nohup — agent picks up new connector YAMLs)
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/sync", h.syncAgent)

	// Install connector on existing host
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/connectors/{connector}/install", h.installConnector)

	// Host management
	mux.HandleFunc("DELETE /api/v1/hosts/{hostname}", h.deleteHost)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/provision", h.hostProvision)
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/recheck/{connector}", h.recheckConnector)

	// Per-host connector removal
	mux.HandleFunc("DELETE /api/v1/hosts/{hostname}/connectors/{connector}", h.deleteHostConnector)

	// Ignore rules
	mux.HandleFunc("POST /api/v1/hosts/{hostname}/ignore/{connector}", h.ignoreConnector)
	mux.HandleFunc("DELETE /api/v1/hosts/{hostname}/ignore/{connector}", h.unignoreConnector)

	// Host provisioning
	mux.HandleFunc("POST /api/v1/provisions", h.createProvision)
	mux.HandleFunc("GET /api/v1/provisions", h.listProvisions)
	mux.HandleFunc("PUT /api/v1/provisions/{token}", h.updateProvision)
	mux.HandleFunc("DELETE /api/v1/provisions/{token}", h.deleteProvision)
	mux.HandleFunc("GET /api/v1/provisions/{token}/config", h.getProvisionConfig)

	// Connector YAML management
	mux.HandleFunc("GET /api/v1/connectors/{name}/yaml", h.getConnectorYAML)
	mux.HandleFunc("PUT /api/v1/connectors/{name}", h.saveConnector)
	mux.HandleFunc("DELETE /api/v1/connectors/{name}", h.deleteConnector)

	// Agent install
	mux.HandleFunc("GET /install", h.serveInstallScript)
	mux.HandleFunc("GET /api/v1/agent/binary/{arch}", h.serveAgentBinary)

	// Notification settings
	mux.HandleFunc("GET /api/v1/settings/notifications", h.getNotificationSettings)
	mux.HandleFunc("PUT /api/v1/settings/notifications", h.saveNotificationSettings)
	mux.HandleFunc("POST /api/v1/settings/notifications/test", h.testNotification)

	mux.HandleFunc("GET /healthz", h.healthz)

	return cors(mux)
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	var req model.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Tell deleted agents to uninstall themselves
	if req.Token == "" && h.store.IsDeleted(req.Hostname) {
		http.Error(w, "host deleted", http.StatusNotFound)
		return
	}

	// If agent reports with a token, claim the provision (clears any tombstone)
	if req.Token != "" {
		h.store.ClaimProvision(req.Token, req.Hostname)
	}

	h.store.Upsert(req)
	log.Printf("report from %s: %d result(s)", req.Hostname, len(req.Results))

	// Notify about new updates (deduped — only fires once per update cycle)
	go func() {
		cfg := h.loadNotificationSettings()
		candidates := h.store.NewUpdates(req.Hostname, cfg.CooldownDays)
		var filtered []store.UpdateEntry
		for _, u := range candidates {
			if cfg.MinCount > 0 {
				if n := store.ParseCount(u.ValuesJSON); n >= 0 && n < cfg.MinCount {
					continue
				}
			}
			filtered = append(filtered, u)
		}
		if len(filtered) > 0 {
			notifyCfg := h.toNotifyConfig(cfg)
			updates := make([]notify.Update, len(filtered))
			for i, u := range filtered {
				updates[i] = notify.Update{Connector: u.Connector, DisplayName: u.DisplayName}
			}
			notify.Send(notifyCfg, req.Hostname, updates)
			h.store.MarkNotified(req.Hostname, filtered)
		}
		h.store.ClearResolved(req.Hostname)
	}()

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) hosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.store.AllHosts())
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) hostDetail(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	status, ok := h.store.GetHostStatus(hostname)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cmds := h.store.HostCommands(hostname)
	if cmds == nil {
		cmds = []model.Command{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"host":     status.Host,
		"results":  status.Results,
		"commands": cmds,
	})
}

func (h *Handler) ignoreConnector(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	connector := r.PathValue("connector")
	item := r.URL.Query().Get("item")
	h.store.SetIgnored(hostname, connector, item, true)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) unignoreConnector(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	connector := r.PathValue("connector")
	item := r.URL.Query().Get("item")
	h.store.SetIgnored(hostname, connector, item, false)
	w.WriteHeader(http.StatusNoContent)
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
