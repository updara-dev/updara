package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/updara/server/model"
	"github.com/updara/server/notify"
	"github.com/updara/server/store"
)

type Handler struct {
	store         *store.Store
	connectorsDir string
	binariesDir   string
	publicURL     string
	authToken     string
}

func NewHandler(s *store.Store, connectorsDir, binariesDir, publicURL, authToken string) *Handler {
	return &Handler{
		store:         s,
		connectorsDir: connectorsDir,
		binariesDir:   binariesDir,
		publicURL:     publicURL,
		authToken:     authToken,
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
	mux.HandleFunc("PATCH /api/v1/hosts/{hostname}/rename", h.renameHost)
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
	mux.HandleFunc("POST /api/v1/settings/notifications/test-digest", h.testDigest)

	// Statistics
	mux.HandleFunc("GET /api/v1/stats", h.globalStats)
	mux.HandleFunc("GET /api/v1/hosts/{hostname}/stats", h.hostStats)

	mux.HandleFunc("GET /healthz", h.healthz)

	return cors(h.authMiddleware(mux))
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || h.isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+h.authToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) isPublicPath(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case r.Method == "POST" && p == "/api/v1/report":
		return true
	case r.Method == "GET" && strings.HasSuffix(p, "/commands/pending"):
		return true
	case r.Method == "POST" && strings.HasPrefix(p, "/api/v1/commands/") && strings.HasSuffix(p, "/result"):
		return true
	case r.Method == "GET" && strings.HasPrefix(p, "/api/v1/provisions/") && strings.HasSuffix(p, "/config"):
		return true
	case r.Method == "GET" && p == "/install":
		return true
	case r.Method == "GET" && strings.HasPrefix(p, "/api/v1/agent/binary/"):
		return true
	case r.Method == "GET" && strings.HasPrefix(p, "/api/v1/connectors/") && strings.HasSuffix(p, "/yaml"):
		return true
	case r.Method == "GET" && p == "/healthz":
		return true
	}
	return false
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

	// If agent reports with a token, use the provision's canonical hostname.
	// This corrects truncated hostnames caused by unquoted systemd env vars
	// (e.g. agent reports "203" but provision says "203 PiHole").
	if req.Token != "" {
		if canonical := h.store.ClaimProvision(req.Token, req.Hostname); canonical != "" {
			req.Hostname = canonical
		}
	}

	cfg := h.loadNotificationSettings()
	if !cfg.ShowLTSUpgrades {
		for i, r := range req.Results {
			if r.Connector == "system-eol" && r.Values["status"] == "upgrade_available" {
				req.Results[i].UpdateAvailable = false
			}
		}
	}
	h.store.Upsert(req)
	log.Printf("report from %s: %d result(s)", req.Hostname, len(req.Results))

	// Notify about new updates (deduped — only fires once per update cycle)
	go func(cfg notificationSettings) {
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
			h.store.MarkNotified(req.Hostname, filtered)
			if cfg.BatchSchedule == "immediate" {
				notifyCfg := h.toNotifyConfig(cfg)
				updates := make([]notify.Update, len(filtered))
				for i, u := range filtered {
					updates[i] = notify.Update{Connector: u.Connector, DisplayName: u.DisplayName}
				}
				notify.Send(notifyCfg, req.Hostname, updates)
			} else {
				h.store.EnqueueNotifications(req.Hostname, filtered)
			}
		}
		h.store.ClearResolved(req.Hostname)
	}(cfg)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) hosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.store.AllHosts())
}

// StartNotificationScheduler runs a background ticker that flushes the
// notification queue at the configured schedule (hourly, daily, twice_daily).
func (h *Handler) StartNotificationScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var lastFired time.Time
		var lastDigestFired time.Time
		for now := range ticker.C {
			cfg := h.loadNotificationSettings()
			nowTrunc := now.Truncate(time.Minute)

			// Digest runs independently of batch schedule
			if !nowTrunc.Equal(lastDigestFired) && h.shouldFireDigest(cfg, now) {
				lastDigestFired = nowTrunc
				go h.sendDigest(cfg) //nolint:errcheck
			}

			if cfg.BatchSchedule == "immediate" {
				continue
			}
			if nowTrunc.Equal(lastFired) {
				continue
			}
			if h.shouldFireBatch(cfg, now) {
				h.flushNotificationQueue(cfg)
				lastFired = nowTrunc
			}
		}
	}()
}

func (h *Handler) shouldFireBatch(cfg notificationSettings, now time.Time) bool {
	switch cfg.BatchSchedule {
	case "hourly":
		return now.Minute() == 0
	case "daily":
		hr, min := parseHHMM(cfg.BatchTime1)
		return now.Hour() == hr && now.Minute() == min
	case "twice_daily":
		hr1, min1 := parseHHMM(cfg.BatchTime1)
		hr2, min2 := parseHHMM(cfg.BatchTime2)
		return (now.Hour() == hr1 && now.Minute() == min1) ||
			(now.Hour() == hr2 && now.Minute() == min2)
	}
	return false
}

func parseHHMM(s string) (int, int) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 7, 0
	}
	hr, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	return hr, min
}

func (h *Handler) flushNotificationQueue(cfg notificationSettings) {
	queued := h.store.FlushQueue()
	if len(queued) == 0 {
		return
	}
	hostnames := make([]string, 0, len(queued))
	for hostname := range queued {
		hostnames = append(hostnames, hostname)
	}
	sort.Strings(hostnames)

	notifyCfg := h.toNotifyConfig(cfg)
	batch := make(map[string][]notify.Update, len(queued))
	for _, hostname := range hostnames {
		entries := queued[hostname]
		updates := make([]notify.Update, len(entries))
		for i, e := range entries {
			updates[i] = notify.Update{Connector: e.Connector, DisplayName: e.DisplayName}
		}
		batch[hostname] = updates
	}
	notify.SendBatch(notifyCfg, batch)
}

func (h *Handler) shouldFireDigest(cfg notificationSettings, now time.Time) bool {
	if !cfg.DigestEnabled || !cfg.EmailEnabled || cfg.EmailHost == "" || cfg.EmailTo == "" {
		return false
	}
	hr, min := parseHHMM(cfg.DigestTime)
	if now.Hour() != hr || now.Minute() != min {
		return false
	}
	switch cfg.DigestFrequency {
	case "daily":
		return true
	case "weekly":
		wd := int(now.Weekday()) // 0=Sun … 6=Sat
		target := cfg.DigestWeekday % 7 // 1=Mon…6=Sat, 7→0=Sun
		return wd == target
	default: // monthly
		day := cfg.DigestDay
		if day < 1 || day > 28 {
			day = 1
		}
		return now.Day() == day
	}
}

func (h *Handler) sendDigest(cfg notificationSettings) error {
	hosts := h.store.DigestSummary()
	now := time.Now()

	var totalUpdates, totalErrors int
	var updateLines, errorLines, hostLines []string

	for _, host := range hosts {
		name := host.DisplayName
		if name == "" {
			name = host.Hostname
		}
		totalUpdates += len(host.Updates)
		totalErrors += len(host.Errors)

		for _, u := range host.Updates {
			since := ""
			if !u.Since.IsZero() {
				days := int(now.Sub(u.Since).Hours() / 24)
				if days == 1 {
					since = " (seit 1 Tag)"
				} else if days > 1 {
					since = fmt.Sprintf(" (seit %d Tagen)", days)
				}
			}
			updateLines = append(updateLines, fmt.Sprintf("  • %s — %s%s", name, u.Name, since))
		}
		for _, e := range host.Errors {
			errorLines = append(errorLines, fmt.Sprintf("  • %s — %s", name, e))
		}

		age := now.Sub(host.LastSeen)
		var ageStr string
		switch {
		case age < time.Minute:
			ageStr = fmt.Sprintf("%ds", int(age.Seconds()))
		case age < time.Hour:
			ageStr = fmt.Sprintf("%dm", int(age.Minutes()))
		case age < 24*time.Hour:
			ageStr = fmt.Sprintf("%dh", int(age.Hours()))
		default:
			ageStr = fmt.Sprintf("%dd", int(age.Hours()/24))
		}
		icon := "✅"
		if len(host.Errors) > 0 {
			icon = "❌"
		} else if len(host.Updates) > 0 {
			icon = "⚠️"
		}
		hostLines = append(hostLines, fmt.Sprintf("%s %-28s %s  (last seen %s ago)", icon, name, host.IPAddress, ageStr))
	}

	subject := fmt.Sprintf("Updara Digest — %s %d", now.Month(), now.Year())

	var sb strings.Builder
	fmt.Fprintf(&sb, "Updara Digest — %s %d\n", now.Month(), now.Year())
	sb.WriteString(strings.Repeat("=", 44) + "\n\n")
	fmt.Fprintf(&sb, "Monitored:       %d hosts\n", len(hosts))
	fmt.Fprintf(&sb, "Pending updates: %d\n", totalUpdates)
	fmt.Fprintf(&sb, "Errors:          %d\n", totalErrors)

	if len(updateLines) > 0 {
		sb.WriteString("\nUPDATES NEEDED\n" + strings.Repeat("-", 20) + "\n")
		sb.WriteString(strings.Join(updateLines, "\n") + "\n")
	}
	if len(errorLines) > 0 {
		sb.WriteString("\nERRORS\n" + strings.Repeat("-", 20) + "\n")
		sb.WriteString(strings.Join(errorLines, "\n") + "\n")
	}
	sb.WriteString("\nALL HOSTS\n" + strings.Repeat("-", 20) + "\n")
	sb.WriteString(strings.Join(hostLines, "\n") + "\n")

	notifyCfg := h.toNotifyConfig(cfg)
	if err := notify.SendDigest(notifyCfg, subject, sb.String()); err != nil {
		log.Printf("digest email failed: %v", err)
		return err
	}
	log.Printf("digest email sent to %s", cfg.EmailTo)
	return nil
}

func (h *Handler) renameHost(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.RenameHost(hostname, strings.TrimSpace(body.DisplayName)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
