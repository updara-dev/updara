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

	EmailEnabled  bool   `json:"email_enabled"`
	EmailHost     string `json:"email_host"`
	EmailPort     string `json:"email_port"`
	EmailUsername string `json:"email_username"`
	EmailPassword string `json:"email_password"`
	EmailFrom     string `json:"email_from"`
	EmailTo       string `json:"email_to"`
	EmailTLS      string `json:"email_tls"` // starttls | ssl | none

	DigestEnabled   bool   `json:"digest_enabled"`
	DigestFrequency string `json:"digest_frequency"` // daily | weekly | monthly
	DigestWeekday   int    `json:"digest_weekday"`   // 1=Mon … 7=Sun (weekly only)
	DigestDay       int    `json:"digest_day"`        // 1–28 (monthly only)
	DigestTime      string `json:"digest_time"`       // HH:MM

	CooldownDays int `json:"cooldown_days"`
	MinCount     int `json:"min_count"`

	BatchSchedule string `json:"batch_schedule"` // immediate | hourly | daily | twice_daily
	BatchTime1    string `json:"batch_time1"`     // HH:MM — daily + twice_daily first time
	BatchTime2    string `json:"batch_time2"`     // HH:MM — twice_daily second time

	ShowLTSUpgrades bool `json:"show_lts_upgrades"`
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

	emailPort := s["email_port"]
	if emailPort == "" {
		emailPort = "587"
	}
	emailTLS := s["email_tls"]
	if emailTLS == "" {
		emailTLS = "starttls"
	}
	digestFrequency := s["digest_frequency"]
	if digestFrequency == "" {
		digestFrequency = "monthly"
	}
	digestWeekday := 1 // Monday default
	if v, err := strconv.Atoi(s["digest_weekday"]); err == nil && v >= 1 && v <= 7 {
		digestWeekday = v
	}
	digestDay := 1
	if v, err := strconv.Atoi(s["digest_day"]); err == nil && v >= 1 && v <= 28 {
		digestDay = v
	}
	digestTime := s["digest_time"]
	if digestTime == "" {
		digestTime = "08:00"
	}

	return notificationSettings{
		NtfyURL:         s["ntfy_url"],
		NtfyTopic:       s["ntfy_topic"],
		NtfyEnabled:     s["ntfy_enabled"] == "true",
		TelegramToken:   s["telegram_token"],
		TelegramChatID:  s["telegram_chat_id"],
		TelegramEnabled: s["telegram_enabled"] == "true",
		EmailEnabled:    s["email_enabled"] == "true",
		EmailHost:       s["email_host"],
		EmailPort:       emailPort,
		EmailUsername:   s["email_username"],
		EmailPassword:   s["email_password"],
		EmailFrom:       s["email_from"],
		EmailTo:         s["email_to"],
		EmailTLS:        emailTLS,
		DigestEnabled:   s["digest_enabled"] == "true",
		DigestFrequency: digestFrequency,
		DigestWeekday:   digestWeekday,
		DigestDay:       digestDay,
		DigestTime:      digestTime,
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
		EmailEnabled:    ns.EmailEnabled,
		EmailHost:       ns.EmailHost,
		EmailPort:       ns.EmailPort,
		EmailUsername:   ns.EmailUsername,
		EmailPassword:   ns.EmailPassword,
		EmailFrom:       ns.EmailFrom,
		EmailTo:         ns.EmailTo,
		EmailTLS:        ns.EmailTLS,
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
	h.store.SetSetting("email_enabled", boolStr(ns.EmailEnabled))
	h.store.SetSetting("email_host", ns.EmailHost)
	h.store.SetSetting("email_port", ns.EmailPort)
	h.store.SetSetting("email_username", ns.EmailUsername)
	h.store.SetSetting("email_password", ns.EmailPassword)
	h.store.SetSetting("email_from", ns.EmailFrom)
	h.store.SetSetting("email_to", ns.EmailTo)
	h.store.SetSetting("email_tls", ns.EmailTLS)
	h.store.SetSetting("digest_enabled", boolStr(ns.DigestEnabled))
	h.store.SetSetting("digest_frequency", ns.DigestFrequency)
	h.store.SetSetting("digest_weekday", strconv.Itoa(ns.DigestWeekday))
	h.store.SetSetting("digest_day", strconv.Itoa(ns.DigestDay))
	h.store.SetSetting("digest_time", ns.DigestTime)
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

// POST /api/v1/settings/notifications/test-digest
func (h *Handler) testDigest(w http.ResponseWriter, r *http.Request) {
	cfg := h.loadNotificationSettings()
	if err := h.sendDigest(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
