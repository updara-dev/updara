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
	"github.com/updara/server/security"
)

const securityRunInterval = 15 * time.Minute

type securitySettings struct {
	Enabled              bool   `json:"enabled"`
	AlertKEV             bool   `json:"alert_kev"`
	AlertSecurityRelease bool   `json:"alert_security_release"`
	AlertEOL             bool   `json:"alert_eol"`
	NotifyResolved       bool   `json:"notify_resolved"`
	DigestEnabled        bool   `json:"digest_enabled"`
	DigestWeekday        int    `json:"digest_weekday"` // 1=Mon … 7=Sun
	DigestTime           string `json:"digest_time"`    // HH:MM
}

func (h *Handler) loadSecuritySettings() securitySettings {
	s := h.store.AllSettings()
	on := func(key string) bool { return s[key] != "false" } // all default to true
	weekday := 1
	if v, err := strconv.Atoi(s["security_digest_weekday"]); err == nil && v >= 1 && v <= 7 {
		weekday = v
	}
	digestTime := s["security_digest_time"]
	if digestTime == "" {
		digestTime = "08:00"
	}
	return securitySettings{
		Enabled:              on("security_enabled"),
		AlertKEV:             on("security_alert_kev"),
		AlertSecurityRelease: on("security_alert_security_release"),
		AlertEOL:             on("security_alert_eol"),
		NotifyResolved:       on("security_notify_resolved"),
		DigestEnabled:        on("security_digest_enabled"),
		DigestWeekday:        weekday,
		DigestTime:           digestTime,
	}
}

// Distro runtimes never alert: distros backport fixes without bumping the
// upstream version, so a version match says little.
func (cfg securitySettings) shouldAlert(f model.SecurityFinding) bool {
	if f.Distro {
		return false
	}
	switch {
	case f.KEV:
		return cfg.AlertKEV
	case f.Kind == "security":
		return cfg.AlertSecurityRelease
	case f.Kind == "eol":
		return cfg.AlertEOL
	}
	return false
}

func (h *Handler) StartSecurityRadar() {
	go func() {
		var lastDigest time.Time
		tick := func(now time.Time) {
			cfg := h.loadSecuritySettings()
			if !cfg.Enabled {
				return
			}
			changed := h.secFeeds.Refresh(false)
			h.secMu.Lock()
			due := changed || now.Sub(h.secLastRun) >= securityRunInterval
			h.secMu.Unlock()
			if due {
				h.runSecurityRadar()
			}
			nowTrunc := now.Truncate(time.Minute)
			if cfg.DigestEnabled && !nowTrunc.Equal(lastDigest) && isDigestTime(cfg, now) {
				lastDigest = nowTrunc
				h.sendSecurityDigest()
			}
		}
		tick(time.Now())
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for now := range ticker.C {
			tick(now)
		}
	}()
}

func isDigestTime(cfg securitySettings, now time.Time) bool {
	hr, min := parseHHMM(cfg.DigestTime)
	return int(now.Weekday()) == cfg.DigestWeekday%7 && now.Hour() == hr && now.Minute() == min
}

func (h *Handler) runSecurityRadar() {
	h.secMu.Lock()
	defer h.secMu.Unlock()
	if !h.secFeeds.Ready() {
		log.Printf("security: feeds not loaded yet, skipping match")
		return
	}
	cfg := h.loadSecuritySettings()
	names := h.store.HostDisplayNames()
	now := time.Now()

	active := map[string]bool{}
	var alerts, cleared []model.SecurityFinding
	for hostID, runtimes := range h.store.InventoryRuntimes() {
		active[hostID] = true
		found := h.secFeeds.Match(security.ParseRuntimes(runtimes), now)
		name := names[hostID]
		if name == "" {
			name = hostID
		}
		for i := range found {
			found[i].HostID = hostID
			found[i].HostName = name
		}
		pending, resolved := h.store.SyncFindings(hostID, found)
		h.markPublic(pending)
		for _, f := range pending {
			if cfg.shouldAlert(f) {
				alerts = append(alerts, f)
			}
		}
		if cfg.NotifyResolved {
			cleared = append(cleared, resolved...)
		}
	}
	h.store.PruneFindings(active)
	h.secLastRun = now

	ncfg := h.toNotifyConfig(h.loadNotificationSettings())
	if len(alerts) > 0 {
		sortFindings(alerts)
		notify.SendMessage(ncfg, "🚨 Updara security alert", formatFindings(alerts, true))
		h.store.MarkFindingsNotified(alerts)
		log.Printf("security: alerted %d finding(s)", len(alerts))
	}
	if len(cleared) > 0 {
		sortFindings(cleared)
		var lines []string
		for _, f := range cleared {
			lines = append(lines, fmt.Sprintf("• %s: %s %s — no longer affected", findingWhere(f), runtimeName(f.Runtime), f.Installed))
		}
		notify.SendMessage(ncfg, "✅ Updara security all-clear", strings.Join(lines, "\n"))
		log.Printf("security: all-clear for %d finding(s)", len(cleared))
	}
}

func (h *Handler) sendSecurityDigest() {
	findings := h.store.OpenFindings()
	h.markPublic(findings)
	sortFindings(findings)
	var kev, eol, distro int
	var relevant []model.SecurityFinding
	for _, f := range findings {
		if f.Distro {
			distro++
			continue
		}
		if f.KEV {
			kev++
		}
		if f.Kind == "eol" {
			eol++
		}
		relevant = append(relevant, f)
	}

	var sb strings.Builder
	if len(relevant) == 0 {
		sb.WriteString("No open findings. 👍\n")
	} else {
		fmt.Fprintf(&sb, "Open findings: %d (actively exploited: %d, end of life: %d)\n\n", len(relevant), kev, eol)
		sb.WriteString(formatFindings(relevant, false) + "\n")
	}
	if distro > 0 {
		fmt.Fprintf(&sb, "\n%d distro-packaged runtime(s) listed on the Security page (no alerts).\n", distro)
	}
	sb.WriteString("\nFeeds:\n")
	for _, st := range h.secFeeds.Status() {
		if st.Error != "" {
			fmt.Fprintf(&sb, "✖ %s — %s\n", st.Name, st.Error)
			continue
		}
		fmt.Fprintf(&sb, "✔ %s — %d entries, updated %s\n", st.Name, st.Entries, st.FetchedAt.Format("2006-01-02 15:04"))
	}
	h.secMu.Lock()
	lastRun := h.secLastRun
	h.secMu.Unlock()
	if !lastRun.IsZero() {
		fmt.Fprintf(&sb, "Last check: %s\n", lastRun.Format("2006-01-02 15:04"))
	}
	sb.WriteString(h.offlineDigestSection(time.Now()))
	notify.SendMessage(h.toNotifyConfig(h.loadNotificationSettings()), "🛡️ Updara security digest", sb.String())
}

func formatFindings(findings []model.SecurityFinding, withLinks bool) string {
	var lines []string
	for _, f := range findings {
		line := fmt.Sprintf("• %s%s: %s %s — %s", publicMark(f.Public), findingWhere(f), runtimeName(f.Runtime), f.Installed, f.Detail)
		if f.Fixed != "" {
			line += " → fix: " + f.Fixed
		}
		if withLinks {
			for i, cve := range f.CVEs {
				if i == 3 {
					break
				}
				line += "\n  https://nvd.nist.gov/vuln/detail/" + cve
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func findingWhere(f model.SecurityFinding) string {
	if f.Location == "host" {
		return f.HostName
	}
	return f.HostName + " · container " + f.Location
}

func runtimeName(r string) string {
	switch r {
	case "node":
		return "Node.js"
	case "python3":
		return "Python"
	case "php":
		return "PHP"
	}
	return r
}

func kindRank(k string) int {
	switch k {
	case "security":
		return 0
	case "kev":
		return 1
	}
	return 2
}

// sortFindings orders by severity: KEV first, then publicly reachable hosts,
// then security releases before EOL; distro runtimes last.
func sortFindings(fs []model.SecurityFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Distro != b.Distro {
			return !a.Distro
		}
		if a.KEV != b.KEV {
			return a.KEV
		}
		if a.Public != b.Public {
			return a.Public
		}
		if kindRank(a.Kind) != kindRank(b.Kind) {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		if a.HostName != b.HostName {
			return a.HostName < b.HostName
		}
		return a.Location < b.Location
	})
}

// GET /api/v1/security
func (h *Handler) securityOverview(w http.ResponseWriter, r *http.Request) {
	findings := h.store.OpenFindings()
	h.markPublic(findings)
	sortFindings(findings)
	h.secMu.Lock()
	lastRun := h.secLastRun
	h.secMu.Unlock()
	resp := map[string]any{
		"enabled":  h.loadSecuritySettings().Enabled,
		"findings": findings,
		"feeds":    h.secFeeds.Status(),
		"hosts":    len(h.store.InventoryRuntimes()),
	}
	if !lastRun.IsZero() {
		resp["last_run"] = lastRun
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// POST /api/v1/security/run — refetch all feeds and match immediately.
func (h *Handler) securityRun(w http.ResponseWriter, r *http.Request) {
	h.secFeeds.Refresh(true)
	h.runSecurityRadar()
	h.securityOverview(w, r)
}

// GET /api/v1/settings/security
func (h *Handler) getSecuritySettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.loadSecuritySettings())
}

// PUT /api/v1/settings/security
func (h *Handler) saveSecuritySettings(w http.ResponseWriter, r *http.Request) {
	var ss securitySettings
	if err := json.NewDecoder(r.Body).Decode(&ss); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	set := func(key string, b bool) { h.store.SetSetting(key, strconv.FormatBool(b)) }
	set("security_enabled", ss.Enabled)
	set("security_alert_kev", ss.AlertKEV)
	set("security_alert_security_release", ss.AlertSecurityRelease)
	set("security_alert_eol", ss.AlertEOL)
	set("security_notify_resolved", ss.NotifyResolved)
	set("security_digest_enabled", ss.DigestEnabled)
	h.store.SetSetting("security_digest_weekday", strconv.Itoa(ss.DigestWeekday))
	h.store.SetSetting("security_digest_time", ss.DigestTime)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/settings/security/test-digest
func (h *Handler) testSecurityDigest(w http.ResponseWriter, r *http.Request) {
	h.sendSecurityDigest()
	w.WriteHeader(http.StatusNoContent)
}
