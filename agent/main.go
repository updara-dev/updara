package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	defaultInterval = 3600
	reportInterval  = 60
	agentVersion    = "0.1.0"
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
	rep := reporter.New(serverURL, hostname, agentVersion, token)

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
