package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/updara/server/notify"
)

type notificationSettings struct {
	NtfyURL     string `json:"ntfy_url"`
	NtfyTopic   string `json:"ntfy_topic"`
	NtfyEnabled bool   `json:"ntfy_enabled"`

	TelegramToken   string `json:"telegram_token"`
	TelegramChatID  string `json:"telegram_chat_id"`
	TelegramEnabled bool   `json:"telegram_enabled"`

	CooldownDays int `json:"cooldown_days"`
	MinCount     int `json:"min_count"`

	BatchSchedule string `json:"batch_schedule"` // immediate | hourly | daily | twice_daily
	BatchTime1    string `json:"batch_time1"`     // HH:MM — daily + twice_daily first time
	BatchTime2    string `json:"batch_time2"`     // HH:MM — twice_daily second time

	ShowLTSUpgrades bool `json:"show_lts_upgrades"` // treat available LTS upgrade as update_available
}

func (h *Handler) loadNotificationSettings() notificationSettings {
	s := h.store.AllSettings()
	cooldown := 3
	if v, err := strconv.Atoi(s["notification_cooldown_days"]); err == nil && v > 0 {
		cooldown = v
	}
	minCount := 0
	if v, err := strconv.Atoi(s["notification_min_count"]); err == nil && v >= 0 {
		minCount = v
	}
	batchSchedule := s["notification_batch_schedule"]
	if batchSchedule == "" {
		batchSchedule = "immediate"
	}
	batchTime1 := s["notification_batch_time1"]
	if batchTime1 == "" {
		batchTime1 = "07:00"
	}
	batchTime2 := s["notification_batch_time2"]
	if batchTime2 == "" {
		batchTime2 = "19:00"
	}
	showLTS := s["show_lts_upgrades"] != "false" // default true
	return notificationSettings{
		NtfyURL:         s["ntfy_url"],
		NtfyTopic:       s["ntfy_topic"],
		NtfyEnabled:     s["ntfy_enabled"] == "true",
		TelegramToken:   s["telegram_token"],
		TelegramChatID:  s["telegram_chat_id"],
		TelegramEnabled: s["telegram_enabled"] == "true",
		CooldownDays:    cooldown,
		MinCount:        minCount,
		BatchSchedule:   batchSchedule,
		BatchTime1:      batchTime1,
		BatchTime2:      batchTime2,
		ShowLTSUpgrades: showLTS,
	}
}

func (h *Handler) toNotifyConfig(ns notificationSettings) notify.Config {
	return notify.Config{
		NtfyURL:         ns.NtfyURL,
		NtfyTopic:       ns.NtfyTopic,
		NtfyEnabled:     ns.NtfyEnabled,
		TelegramToken:   ns.TelegramToken,
		TelegramChatID:  ns.TelegramChatID,
		TelegramEnabled: ns.TelegramEnabled,
	}
}

// GET /api/v1/settings/notifications
func (h *Handler) getNotificationSettings(w http.ResponseWriter, r *http.Request) {
	ns := h.loadNotificationSettings()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ns)
}

// PUT /api/v1/settings/notifications
func (h *Handler) saveNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var ns notificationSettings
	if err := json.NewDecoder(r.Body).Decode(&ns); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}
	h.store.SetSetting("ntfy_url", ns.NtfyURL)
	h.store.SetSetting("ntfy_topic", ns.NtfyTopic)
	h.store.SetSetting("ntfy_enabled", boolStr(ns.NtfyEnabled))
	h.store.SetSetting("telegram_token", ns.TelegramToken)
	h.store.SetSetting("telegram_chat_id", ns.TelegramChatID)
	h.store.SetSetting("telegram_enabled", boolStr(ns.TelegramEnabled))
	h.store.SetSetting("notification_cooldown_days", strconv.Itoa(ns.CooldownDays))
	h.store.SetSetting("notification_min_count", strconv.Itoa(ns.MinCount))
	h.store.SetSetting("notification_batch_schedule", ns.BatchSchedule)
	h.store.SetSetting("notification_batch_time1", ns.BatchTime1)
	h.store.SetSetting("notification_batch_time2", ns.BatchTime2)
	h.store.SetSetting("show_lts_upgrades", boolStr(ns.ShowLTSUpgrades))
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/settings/notifications/test
func (h *Handler) testNotification(w http.ResponseWriter, r *http.Request) {
	cfg := h.toNotifyConfig(h.loadNotificationSettings())
	if err := notify.SendTest(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
