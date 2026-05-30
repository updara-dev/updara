package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/updara/agent/connector"
	"github.com/updara/agent/reporter"
	"github.com/updara/agent/runner"
)

const (
	defaultInterval  = 3600
	reportInterval   = 60
	syncInterval     = 3600 // check for connector updates every hour
	agentVersion     = "0.1.0"
)

func main() {
	serverURL := env("SERVER_URL", "http://localhost:8080")
	connDir := env("CONNECTORS_DIR", "/etc/updara/connectors")
	token := os.Getenv("UPDARA_TOKEN")

	// Bootstrap: fetch connector configs from server if token is set
	if token != "" {
		if err := bootstrap(serverURL, token, connDir); err != nil {
			log.Printf("bootstrap failed: %v — continuing with local connectors", err)
		}
	}

	hostname := env("HOSTNAME_OVERRIDE", mustHostname())
	ip := localIP()
	rep := reporter.New(serverURL, hostname, ip, agentVersion, token)

	connectors, err := connector.LoadAll(connDir)
	if err != nil {
		log.Fatalf("load connectors: %v", err)
	}
	log.Printf("Loaded %d connector(s) from %s", len(connectors), connDir)

	var mu sync.Mutex
	latest := make(map[string]runner.Result)

	for _, c := range connectors {
		c := c
		go func() {
			interval := c.Check.Interval
			if interval <= 0 {
				interval = defaultInterval
			}
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				result := runner.Run(ctx, c)
				cancel()

				if result.Error != "" {
					log.Printf("[%s] error: %s", c.Name, result.Error)
				} else {
					log.Printf("[%s] update_available=%v values=%v", c.Name, result.UpdateAvailable, result.Values)
				}

				mu.Lock()
				latest[c.Name] = result
				mu.Unlock()

				time.Sleep(time.Duration(interval) * time.Second)
			}
		}()
	}

	// Sync connector YAMLs from server on startup (after 10s) and every hour.
	// Also discovers new connectors added to the server after provisioning.
	// If anything changed, exit 0 so systemd restarts with fresh connectors.
	go func() {
		time.Sleep(10 * time.Second)
		for {
			changed := syncConnectors(serverURL, connDir, connectors)
			changed += discoverConnectors(serverURL, connDir)
			if changed > 0 {
				log.Printf("connector sync: %d connector(s) updated/added — restarting", changed)
				os.Exit(0)
			}
			time.Sleep(time.Duration(syncInterval) * time.Second)
		}
	}()

	// Poll for pending commands every 3s.
	// After a successful update, immediately re-check the affected connector.
	go func() {
		for {
			time.Sleep(3 * time.Second)
			if err := pollCommands(serverURL, hostname, connectors, &mu, latest, rep); err != nil {
				log.Printf("command poll: %v", err)
			}
		}
	}()

	// First report after 3s, then every reportInterval seconds
	time.Sleep(3 * time.Second)
	for {
		mu.Lock()
		results := make([]runner.Result, 0, len(latest))
		for _, r := range latest {
			results = append(results, r)
		}
		mu.Unlock()

		if len(results) > 0 {
			if err := rep.Send(results); err != nil {
				if errors.Is(err, reporter.ErrHostDeleted) {
					selfUninstall()
				}
				log.Printf("report failed: %v", err)
			} else {
				log.Printf("Reported %d result(s) to %s", len(results), serverURL)
			}
		}
		time.Sleep(reportInterval * time.Second)
	}
}

// pollCommands fetches pending commands, executes them, and re-checks the
// affected connector immediately after a successful update.
func pollCommands(
	serverURL, hostname string,
	connectors []connector.Connector,
	mu *sync.Mutex,
	latest map[string]runner.Result,
	rep *reporter.Reporter,
) error {
	url := fmt.Sprintf("%s/api/v1/hosts/%s/commands/pending", serverURL, hostname)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var cmds []struct {
		ID        string `json:"id"`
		Connector string `json:"connector"`
		Cmd       string `json:"cmd"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cmds); err != nil {
		return err
	}

	for _, cmd := range cmds {
		log.Printf("[cmd:%s] executing: %s", cmd.ID, cmd.Cmd)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, execErr := exec.CommandContext(ctx, "sh", "-c", cmd.Cmd).CombinedOutput()
		cancel()

		status := "done"
		if execErr != nil {
			status = "failed"
		}
		log.Printf("[cmd:%s] %s", cmd.ID, status)

		body, _ := json.Marshal(map[string]string{"status": status, "output": string(out)})
		http.Post(serverURL+"/api/v1/commands/"+cmd.ID+"/result", "application/json", bytes.NewReader(body))

		// Re-check the affected connector immediately so the dashboard updates at once
		if status == "done" {
			for _, c := range connectors {
				if c.Name != cmd.Connector {
					continue
				}
				ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
				result := runner.Run(ctx2, c)
				cancel2()
				log.Printf("[%s] re-check after update: update_available=%v values=%v",
					c.Name, result.UpdateAvailable, result.Values)
				mu.Lock()
				latest[c.Name] = result
				mu.Unlock()
				break
			}
			// Force a fresh report so the dashboard sees the new state immediately
			mu.Lock()
			results := make([]runner.Result, 0, len(latest))
			for _, r := range latest {
				results = append(results, r)
			}
			mu.Unlock()
			if err := rep.Send(results); err != nil {
				log.Printf("post-update report: %v", err)
			}
		}
	}
	return nil
}

// bootstrap fetches the agent's connector configuration from the server.
func bootstrap(serverURL, token, connDir string) error {
	url := fmt.Sprintf("%s/api/v1/provisions/%s/config", serverURL, token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("token not found on server")
	}

	var cfg struct {
		Hostname   string `json:"hostname"`
		Connectors []struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"connectors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return err
	}

	// Override hostname with provisioned name
	if cfg.Hostname != "" {
		os.Setenv("HOSTNAME_OVERRIDE", cfg.Hostname)
	}

	// Write connector YAMLs to disk
	if err := os.MkdirAll(connDir, 0755); err != nil {
		return err
	}
	for _, c := range cfg.Connectors {
		path := filepath.Join(connDir, c.Name+".yaml")
		if err := os.WriteFile(path, []byte(c.Content), 0644); err != nil {
			log.Printf("bootstrap: write %s: %v", c.Name, err)
		} else {
			log.Printf("bootstrap: installed connector %s", c.Name)
		}
	}

	log.Printf("bootstrap: provisioned as '%s' with %d connector(s)", cfg.Hostname, len(cfg.Connectors))
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// syncConnectors fetches each connector YAML from the server, compares with
// the local file, and overwrites any that differ. Returns the number changed.
func syncConnectors(serverURL, connDir string, connectors []connector.Connector) int {
	client := &http.Client{Timeout: 15 * time.Second}
	changed := 0
	for _, c := range connectors {
		url := fmt.Sprintf("%s/api/v1/connectors/%s/yaml", serverURL, c.Name)
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("connector sync: fetch %s: %v", c.Name, err)
			continue
		}
		remote, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("connector sync: read %s: %v", c.Name, err)
			continue
		}
		// Server explicitly deleted this connector — safe to remove local copy
		if resp.StatusCode == http.StatusNotFound {
			localPath := filepath.Join(connDir, c.Name+".yaml")
			os.Remove(localPath)
			log.Printf("connector sync: removed %s (deleted on server)", c.Name)
			changed++
			continue
		}
		if resp.StatusCode != http.StatusOK {
			log.Printf("connector sync: bad response for %s: status=%d", c.Name, resp.StatusCode)
			continue
		}
		localPath := filepath.Join(connDir, c.Name+".yaml")
		local, err := os.ReadFile(localPath)
		if err != nil || !bytes.Equal(local, remote) {
			if writeErr := os.WriteFile(localPath, remote, 0644); writeErr != nil {
				log.Printf("connector sync: write %s: %v", c.Name, writeErr)
				continue
			}
			log.Printf("connector sync: updated %s", c.Name)
			changed++
		}
	}
	return changed
}

func selfUninstall() {
	log.Printf("host deleted on server — uninstalling agent")
	exec.Command("systemctl", "disable", "updara-agent").Run()
	os.Remove("/etc/systemd/system/updara-agent.service")
	exec.Command("systemctl", "daemon-reload").Run()
	os.RemoveAll("/etc/updara")
	// Remove own binary after exit (run in background so this process can exit cleanly)
	exe, _ := os.Executable()
	exec.Command("sh", "-c", fmt.Sprintf("sleep 2; rm -f %q", exe)).Start()
	log.Printf("uninstall complete")
	os.Exit(0)
}

// discoverConnectors downloads connector YAMLs that exist on the server but
// are not yet present on disk. Called after syncConnectors each cycle.
func discoverConnectors(serverURL, connDir string) int {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(serverURL + "/api/v1/connectors")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var meta []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return 0
	}
	added := 0
	for _, m := range meta {
		localPath := filepath.Join(connDir, m.Name+".yaml")
		if _, err := os.Stat(localPath); err == nil {
			continue // already exists
		}
		yamlResp, err := client.Get(fmt.Sprintf("%s/api/v1/connectors/%s/yaml", serverURL, m.Name))
		if err != nil || yamlResp.StatusCode != http.StatusOK {
			if yamlResp != nil {
				yamlResp.Body.Close()
			}
			continue
		}
		data, _ := io.ReadAll(yamlResp.Body)
		yamlResp.Body.Close()
		if writeErr := os.WriteFile(localPath, data, 0644); writeErr == nil {
			log.Printf("connector sync: discovered new connector %s", m.Name)
			added++
		}
	}
	return added
}

func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
