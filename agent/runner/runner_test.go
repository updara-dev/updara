package runner

import (
	"context"
	"testing"

	"github.com/updara/agent/connector"
)

func shellConnector(cmd string) connector.Connector {
	return connector.Connector{
		Name: "test",
		Check: connector.Check{
			Type:            "shell",
			Command:         cmd,
			Parse:           map[string]string{"current": "$current", "latest": "$latest"},
			UpdateAvailable: "current != latest",
		},
	}
}

func TestRunErrorLine(t *testing.T) {
	r := Run(context.Background(), shellConnector(`echo "error=Immich API: connection refused"`))
	if r.Error != "Immich API: connection refused" || r.UpdateAvailable {
		t.Fatalf("got error=%q update=%v", r.Error, r.UpdateAvailable)
	}
}

func TestRunMissingValue(t *testing.T) {
	r := Run(context.Background(), shellConnector(`echo current=v1.0.0`))
	if r.Error == "" || r.UpdateAvailable {
		t.Fatalf("expected error for missing latest, got error=%q update=%v", r.Error, r.UpdateAvailable)
	}
}

func TestRunUpToDate(t *testing.T) {
	r := Run(context.Background(), shellConnector(`printf 'current=v3.2.2\nlatest=v3.2.2\n'`))
	if r.Error != "" || r.UpdateAvailable {
		t.Fatalf("got error=%q update=%v", r.Error, r.UpdateAvailable)
	}
}

func TestRunUpdateAvailable(t *testing.T) {
	r := Run(context.Background(), shellConnector(`printf 'current=v3.1.0\nlatest=v3.2.2\n'`))
	if r.Error != "" || !r.UpdateAvailable {
		t.Fatalf("got error=%q update=%v", r.Error, r.UpdateAvailable)
	}
}

func TestRunLiteralOperand(t *testing.T) {
	c := connector.Connector{Check: connector.Check{
		Type:            "shell",
		Command:         `echo needs_update=true`,
		Parse:           map[string]string{"needs_update": "$needs_update"},
		UpdateAvailable: "needs_update == true",
	}}
	r := Run(context.Background(), c)
	if r.Error != "" || !r.UpdateAvailable {
		t.Fatalf("got error=%q update=%v", r.Error, r.UpdateAvailable)
	}
}
