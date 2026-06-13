package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
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

	EmailEnabled  bool
	EmailHost     string
	EmailPort     string // default "587"
	EmailUsername string
	EmailPassword string
	EmailFrom     string
	EmailTo       string
	EmailTLS      string // "starttls" | "ssl" | "none"
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
	if cfg.EmailEnabled && cfg.EmailHost != "" && cfg.EmailTo != "" {
		go sendEmail(cfg, title, body)
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
	if cfg.EmailEnabled && cfg.EmailHost != "" && cfg.EmailTo != "" {
		go sendEmail(cfg, title, body)
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
	if cfg.EmailEnabled && cfg.EmailHost != "" && cfg.EmailTo != "" {
		if err := sendEmail(cfg, title, body); err != nil {
			errs = append(errs, "email: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// SendDigest sends a digest email with pre-built subject and body.
func SendDigest(cfg Config, subject, body string) error {
	if !cfg.EmailEnabled || cfg.EmailHost == "" || cfg.EmailTo == "" {
		return nil
	}
	return sendEmail(cfg, subject, body)
}

func sendEmail(cfg Config, subject, body string) error {
	port := cfg.EmailPort
	if port == "" {
		port = "587"
	}
	addr := cfg.EmailHost + ":" + port

	from := cfg.EmailFrom
	if from == "" {
		from = cfg.EmailUsername
	}

	header := "From: " + from + "\r\n" +
		"To: " + cfg.EmailTo + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n"
	msg := []byte(header + body)

	var auth smtp.Auth
	if cfg.EmailUsername != "" {
		auth = smtp.PlainAuth("", cfg.EmailUsername, cfg.EmailPassword, cfg.EmailHost)
	}

	if cfg.EmailTLS == "ssl" {
		return sendEmailSSL(addr, cfg.EmailHost, auth, from, cfg.EmailTo, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{cfg.EmailTo}, msg)
}

func sendEmailSSL(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err = c.Auth(auth); err != nil {
			return err
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	if err = c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	return w.Close()
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
