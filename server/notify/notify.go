package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Config struct {
	NtfyURL     string
	NtfyTopic   string
	NtfyEnabled bool

	TelegramToken   string
	TelegramChatID  string
	TelegramEnabled bool
}

type Update struct {
	Connector   string
	DisplayName string
}

func Send(cfg Config, hostname string, updates []Update) {
	if len(updates) == 0 {
		return
	}

	lines := make([]string, len(updates))
	for i, u := range updates {
		name := u.DisplayName
		if name == "" {
			name = u.Connector
		}
		lines[i] = "• " + name
	}

	title := fmt.Sprintf("Updates available — %s", hostname)
	body := strings.Join(lines, "\n")

	if cfg.NtfyEnabled && cfg.NtfyURL != "" && cfg.NtfyTopic != "" {
		go sendNtfy(cfg.NtfyURL, cfg.NtfyTopic, title, body)
	}
	if cfg.TelegramEnabled && cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		go sendTelegram(cfg.TelegramToken, cfg.TelegramChatID, title+"\n"+body)
	}
}

// SendBatch sends a single notification summarising updates across multiple hosts.
func SendBatch(cfg Config, batch map[string][]Update) {
	if len(batch) == 0 {
		return
	}
	hostnames := make([]string, 0, len(batch))
	for h := range batch {
		hostnames = append(hostnames, h)
	}
	sort.Strings(hostnames)

	var sb strings.Builder
	for _, hostname := range hostnames {
		sb.WriteString(hostname + "\n")
		for _, u := range batch[hostname] {
			name := u.DisplayName
			if name == "" {
				name = u.Connector
			}
			sb.WriteString("  • " + name + "\n")
		}
	}

	title := fmt.Sprintf("Updates available — %d host(s)", len(batch))
	body := strings.TrimRight(sb.String(), "\n")

	if cfg.NtfyEnabled && cfg.NtfyURL != "" && cfg.NtfyTopic != "" {
		go sendNtfy(cfg.NtfyURL, cfg.NtfyTopic, title, body)
	}
	if cfg.TelegramEnabled && cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		go sendTelegram(cfg.TelegramToken, cfg.TelegramChatID, title+"\n"+body)
	}
}

func SendTest(cfg Config) error {
	const title = "Updara — Test Notification"
	const body = "Notifications are working correctly."

	var errs []string
	if cfg.NtfyEnabled && cfg.NtfyURL != "" && cfg.NtfyTopic != "" {
		if err := sendNtfy(cfg.NtfyURL, cfg.NtfyTopic, title, body); err != nil {
			errs = append(errs, "ntfy: "+err.Error())
		}
	}
	if cfg.TelegramEnabled && cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		if err := sendTelegram(cfg.TelegramToken, cfg.TelegramChatID, title+"\n"+body); err != nil {
			errs = append(errs, "telegram: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func sendNtfy(baseURL, topic, title, message string) error {
	url := strings.TrimRight(baseURL, "/") + "/" + topic
	req, err := http.NewRequest("POST", url, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Tags", "bell")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func sendTelegram(token, chatID, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    text,
	})
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned HTTP %d", resp.StatusCode)
	}
	return nil
}
