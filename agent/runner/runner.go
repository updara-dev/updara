package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/updara/agent/connector"
)

type Result struct {
	Connector       string            `json:"connector"`
	DisplayName     string            `json:"display_name"`
	Category        string            `json:"category"`
	Values          map[string]string `json:"values"`
	UpdateAvailable bool              `json:"update_available"`
	Changelog       string            `json:"changelog"`
	Error           string            `json:"error,omitempty"`
	CheckedAt       time.Time         `json:"checked_at"`
}

var envPattern = regexp.MustCompile(`\{([A-Z][A-Z0-9_]*)\}`)

func Run(ctx context.Context, c connector.Connector) Result {
	r := Result{
		Connector:   c.Name,
		DisplayName: c.DisplayName,
		Category:    c.Category,
		Changelog:   c.Notifications.Changelog,
		CheckedAt:   time.Now(),
	}

	var (
		values map[string]string
		err    error
	)

	switch c.Check.Type {
	case "shell":
		values, err = runShell(ctx, c.Check)
	case "http":
		values, err = runHTTP(ctx, c.Check)
	default:
		r.Error = fmt.Sprintf("unknown check type: %q", c.Check.Type)
		return r
	}

	if err != nil {
		r.Error = err.Error()
		return r
	}

	r.Values = values
	r.UpdateAvailable = evalExpr(c.Check.UpdateAvailable, values)
	return r
}

// substituteEnv replaces {VAR} with the corresponding environment variable.
func substituteEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		key := m[1 : len(m)-1]
		if v := os.Getenv(key); v != "" {
			return v
		}
		return m
	})
}

func runShell(ctx context.Context, c connector.Check) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Command)
	out, err := cmd.Output()
	stdout := strings.TrimSpace(string(out))
	if err != nil && stdout == "" {
		return nil, err
	}

	// Build lookup: $stdout + $key for every "key=value" line in output.
	lineMap := map[string]string{"$stdout": stdout}
	for _, line := range strings.Split(stdout, "\n") {
		if idx := strings.Index(line, "="); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			lineMap["$"+k] = v
		}
	}

	values := make(map[string]string)
	for key, spec := range c.Parse {
		if v, ok := lineMap[spec]; ok {
			values[key] = v
		}
	}
	return values, nil
}

func runHTTP(ctx context.Context, c connector.Check) (map[string]string, error) {
	endpoint := substituteEnv(c.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range c.Headers {
		req.Header.Set(k, substituteEnv(v))
	}
	if c.Auth.Type == "bearer" {
		req.Header.Set("Authorization", "Bearer "+substituteEnv(c.Auth.Token))
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	values := make(map[string]string)
	for key, path := range c.Parse {
		if val, err := extractPath(data, path); err == nil {
			values[key] = val
		}
	}
	return values, nil
}

// extractPath resolves simple $.field.sub JSONPath expressions.
func extractPath(data interface{}, path string) (string, error) {
	if !strings.HasPrefix(path, "$.") {
		return "", fmt.Errorf("unsupported path %q (must start with $.)", path)
	}
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	cur := data
	for _, p := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("not an object at %q", p)
		}
		cur, ok = m[p]
		if !ok {
			return "", fmt.Errorf("key %q not found", p)
		}
	}
	return fmt.Sprintf("%v", cur), nil
}

// evalExpr evaluates simple comparison expressions:
//
//	"count > 0"        numeric greater-than
//	"current != latest" string inequality
func evalExpr(expr string, values map[string]string) bool {
	for _, op := range []string{"!=", "==", ">=", "<=", ">", "<"} {
		idx := strings.Index(expr, op)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+len(op):])
		lv := resolve(left, values)
		rv := resolve(right, values)

		switch op {
		case "!=":
			return lv != rv
		case "==":
			return lv == rv
		}
		lf, err1 := strconv.ParseFloat(lv, 64)
		rf, err2 := strconv.ParseFloat(rv, 64)
		if err1 != nil || err2 != nil {
			return false
		}
		switch op {
		case ">":
			return lf > rf
		case ">=":
			return lf >= rf
		case "<":
			return lf < rf
		case "<=":
			return lf <= rf
		}
	}
	return false
}

func resolve(token string, values map[string]string) string {
	if v, ok := values[token]; ok {
		return v
	}
	return token
}
