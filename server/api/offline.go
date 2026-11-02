package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/updara/server/model"
	"github.com/updara/server/notify"
	"github.com/updara/server/store"
)

type offlineSettings struct {
	Enabled              bool `json:"enabled"`
	ThresholdHours       int  `json:"threshold_hours"`
	PublicThresholdHours int  `json:"public_threshold_hours"`
}

func (h *Handler) loadOfflineSettings() offlineSettings {
	s := h.store.AllSettings()
	hours := func(key string, def int) int {
		if v, err := strconv.Atoi(s[key]); err == nil && v > 0 {
			return v
		}
		return def
	}
	return offlineSettings{
		Enabled:              s["offline_enabled"] != "false",
		ThresholdHours:       hours("offline_threshold_hours", 24),
		PublicThresholdHours: hours("offline_public_threshold_hours", 1),
	}
}

func (cfg offlineSettings) threshold(public bool) time.Duration {
	if public {
		return time.Duration(cfg.PublicThresholdHours) * time.Hour
	}
	return time.Duration(cfg.ThresholdHours) * time.Hour
}

func (h *Handler) StartOfflineWatch() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for now := range ticker.C {
			h.checkOfflineAgents(now)
		}
	}()
}

func (h *Handler) checkOfflineAgents(now time.Time) {
	cfg := h.loadOfflineSettings()
	if !cfg.Enabled {
		return
	}
	var gone, back []store.AgentState
	for _, a := range h.store.AgentStates() {
		silent := now.Sub(a.LastSeen)
		switch {
		case a.NotifiedAt == "" && silent > cfg.threshold(a.Public) && !a.Muted(now):
			gone = append(gone, a)
			h.store.SetOfflineNotified(a.ID, now)
		// Agents report every minute; a fresh report means it is back.
		case a.NotifiedAt != "" && silent < 5*time.Minute:
			back = append(back, a)
			h.store.SetOfflineNotified(a.ID, time.Time{})
		}
	}
	ncfg := h.toNotifyConfig(h.loadNotificationSettings())
	if len(gone) > 0 {
		var lines []string
		for _, a := range gone {
			lines = append(lines, "• "+agentLine(a, now))
		}
		notify.SendMessage(ncfg, "📴 Updara: agent offline", strings.Join(lines, "\n"))
		log.Printf("offline: alerted %d host(s)", len(gone))
	}
	if len(back) > 0 {
		var lines []string
		for _, a := range back {
			lines = append(lines, "• "+publicMark(a.Public)+a.Name)
		}
		notify.SendMessage(ncfg, "✅ Updara: agent back online", strings.Join(lines, "\n"))
		log.Printf("offline: %d host(s) back online", len(back))
	}
}

func publicMark(public bool) string {
	if public {
		return "🌐 "
	}
	return ""
}

func agentLine(a store.AgentState, now time.Time) string {
	return fmt.Sprintf("%s%s — last seen %s (%s ago)", publicMark(a.Public), a.Name,
		a.LastSeen.Local().Format("2006-01-02 15:04"), humanDuration(now.Sub(a.LastSeen)))
}

func humanDuration(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d min", int(d.Minutes()))
}

// offlineDigestSection lists silent and muted agents for the weekly digest.
func (h *Handler) offlineDigestSection(now time.Time) string {
	cfg := h.loadOfflineSettings()
	var offline, muted []string
	for _, a := range h.store.AgentStates() {
		if a.Muted(now) {
			until := "indefinitely"
			if a.MutedUntil != "forever" {
				if t, err := time.Parse(time.RFC3339, a.MutedUntil); err == nil {
					until = "until " + t.Local().Format("2006-01-02")
				}
			}
			muted = append(muted, fmt.Sprintf("• %s — muted %s, last seen %s", a.Name, until, a.LastSeen.Local().Format("2006-01-02")))
			continue
		}
		if now.Sub(a.LastSeen) > cfg.threshold(a.Public) {
			offline = append(offline, "• "+agentLine(a, now))
		}
	}
	var sb strings.Builder
	if len(offline) > 0 {
		sb.WriteString("\nAgents offline:\n" + strings.Join(offline, "\n") + "\n")
	}
	if len(muted) > 0 {
		sb.WriteString("\nOffline alert muted:\n" + strings.Join(muted, "\n") + "\n")
	}
	return sb.String()
}

// GET /api/v1/settings/offline
func (h *Handler) getOfflineSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.loadOfflineSettings())
}

// PUT /api/v1/settings/offline
func (h *Handler) saveOfflineSettings(w http.ResponseWriter, r *http.Request) {
	var cfg offlineSettings
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cfg.ThresholdHours < 1 || cfg.PublicThresholdHours < 1 {
		http.Error(w, "thresholds must be at least 1 hour", http.StatusBadRequest)
		return
	}
	h.store.SetSetting("offline_enabled", strconv.FormatBool(cfg.Enabled))
	h.store.SetSetting("offline_threshold_hours", strconv.Itoa(cfg.ThresholdHours))
	h.store.SetSetting("offline_public_threshold_hours", strconv.Itoa(cfg.PublicThresholdHours))
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/v1/hosts/{hostname}/monitoring
func (h *Handler) setHostMonitoring(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Public            bool   `json:"public"`
		OfflineMutedUntil string `json:"offline_muted_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if m := body.OfflineMutedUntil; m != "" && m != "forever" {
		if _, err := time.Parse(time.RFC3339, m); err != nil {
			http.Error(w, "offline_muted_until: expected \"\", \"forever\" or RFC3339", http.StatusBadRequest)
			return
		}
	}
	if err := h.store.SetHostMonitoring(r.PathValue("hostname"), body.Public, body.OfflineMutedUntil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markPublic flags findings on hosts marked as publicly reachable.
func (h *Handler) markPublic(fs []model.SecurityFinding) {
	public := h.store.PublicHosts()
	for i := range fs {
		fs[i].Public = public[fs[i].HostID]
	}
}
