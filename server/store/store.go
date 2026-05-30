package store

import (
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/updara/server/model"
)

// Store wraps a SQLite database. A single mutex serialises all writes;
// reads use the same lock for simplicity (WAL mode keeps read latency low).
type Store struct {
	mu sync.Mutex
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS hosts (
    id            TEXT PRIMARY KEY,
    hostname      TEXT NOT NULL,
    ip_address    TEXT NOT NULL DEFAULT '',
    agent_version TEXT NOT NULL,
    last_seen     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS results (
    host_id         TEXT    NOT NULL,
    connector       TEXT    NOT NULL,
    display_name    TEXT    NOT NULL DEFAULT '',
    category        TEXT    NOT NULL DEFAULT '',
    values_json     TEXT    NOT NULL DEFAULT '{}',
    update_available INTEGER NOT NULL DEFAULT 0,
    changelog       TEXT    NOT NULL DEFAULT '',
    error           TEXT    NOT NULL DEFAULT '',
    checked_at      TEXT    NOT NULL,
    PRIMARY KEY (host_id, connector)
);

CREATE TABLE IF NOT EXISTS provisions (
    token           TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    host_type       TEXT NOT NULL DEFAULT '',
    connectors_json TEXT NOT NULL DEFAULT '[]',
    created_at      TEXT NOT NULL,
    claimed_by      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS commands (
    id         TEXT PRIMARY KEY,
    host_id    TEXT NOT NULL,
    connector  TEXT NOT NULL,
    cmd        TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    output     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ignored_items (
    host_id    TEXT NOT NULL,
    connector  TEXT NOT NULL,
    item       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (host_id, connector, item)
);

CREATE INDEX IF NOT EXISTS idx_results_host    ON results(host_id);
CREATE INDEX IF NOT EXISTS idx_commands_host   ON commands(host_id);
CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
CREATE INDEX IF NOT EXISTS idx_ignored_host    ON ignored_items(host_id);
`

func New(path string) *Store {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_foreign_keys=on")
	if err != nil {
		log.Fatalf("store: open %s: %v", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("store: migrate: %v", err)
	}
	// Incremental migrations — safe to run repeatedly
	migrations := []string{
		`ALTER TABLE hosts ADD COLUMN ip_address TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		db.Exec(m) // ignore errors (column may already exist)
	}

	// Migrate ignored_items to schema with 'item' column (one-time, safe to re-check)
	var itemColCount int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('ignored_items') WHERE name='item'`).Scan(&itemColCount)
	if itemColCount == 0 {
		db.Exec(`DROP TABLE IF EXISTS ignored_items`)
		db.Exec(`CREATE TABLE IF NOT EXISTS ignored_items (host_id TEXT NOT NULL, connector TEXT NOT NULL, item TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, PRIMARY KEY (host_id, connector, item))`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_ignored_host ON ignored_items(host_id)`)
		log.Printf("store: migrated ignored_items to item-level schema")
	}

	log.Printf("store: opened %s", path)
	return &Store{db: db}
}

// ── Hosts ────────────────────────────────────────────────────────────────────

func (s *Store) Upsert(req model.ReportRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO hosts (id, hostname, ip_address, agent_version, last_seen) VALUES (?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			ip_address=excluded.ip_address,
			agent_version=excluded.agent_version,
			last_seen=excluded.last_seen
	`, req.Hostname, req.Hostname, req.IPAddress, req.AgentVersion, now)
	if err != nil {
		log.Printf("store: upsert host: %v", err)
		return
	}

	s.db.Exec(`DELETE FROM results WHERE host_id = ?`, req.Hostname)
	for _, r := range req.Results {
		valJSON, _ := json.Marshal(r.Values)
		upd := 0
		if r.UpdateAvailable {
			upd = 1
		}
		s.db.Exec(`
			INSERT INTO results
			  (host_id,connector,display_name,category,values_json,update_available,changelog,error,checked_at)
			VALUES (?,?,?,?,?,?,?,?,?)
		`, req.Hostname, r.Connector, r.DisplayName, r.Category, string(valJSON),
			upd, r.Changelog, r.Error, r.CheckedAt.UTC().Format(time.RFC3339Nano))
	}
}

func (s *Store) AllHosts() []model.HostStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id, hostname, ip_address, agent_version, last_seen FROM hosts ORDER BY hostname`)
	if err != nil {
		log.Printf("store: query hosts: %v", err)
		return nil
	}

	// Collect all hosts first, then close rows before making nested queries.
	// (With MaxOpenConns=1, a nested Query while rows is open causes a deadlock.)
	var hosts []model.Host
	for rows.Next() {
		var h model.Host
		var lastSeen string
		if err := rows.Scan(&h.ID, &h.Hostname, &h.IPAddress, &h.AgentVersion, &lastSeen); err != nil {
			continue
		}
		h.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
		hosts = append(hosts, h)
	}
	rows.Close()

	var out []model.HostStatus
	for _, h := range hosts {
		ignored := s.fetchIgnored(h.ID)
		out = append(out, model.HostStatus{Host: h, Results: s.queryResults(h.ID, ignored)})
	}
	return out
}

func (s *Store) GetHostStatus(hostname string) (*model.HostStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var h model.Host
	var lastSeen string
	err := s.db.QueryRow(`SELECT id, hostname, ip_address, agent_version, last_seen FROM hosts WHERE id=?`, hostname).
		Scan(&h.ID, &h.Hostname, &h.IPAddress, &h.AgentVersion, &lastSeen)
	if err != nil {
		return nil, false
	}
	h.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)

	ignored := s.fetchIgnored(h.ID)
	return &model.HostStatus{Host: h, Results: s.queryResults(h.ID, ignored)}, true
}

// fetchIgnored must be called while s.mu is held.
// Returns map[connector][]items where item="" means connector-level ignore.
func (s *Store) fetchIgnored(hostID string) map[string][]string {
	rows, err := s.db.Query(`SELECT connector, item FROM ignored_items WHERE host_id=?`, hostID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var connector, item string
		rows.Scan(&connector, &item)
		out[connector] = append(out[connector], item)
	}
	return out
}

func (s *Store) SetIgnored(hostID, connector, item string, ignored bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ignored {
		s.db.Exec(`INSERT OR REPLACE INTO ignored_items (host_id, connector, item, created_at) VALUES (?,?,?,?)`,
			hostID, connector, item, time.Now().UTC().Format(time.RFC3339Nano))
	} else {
		s.db.Exec(`DELETE FROM ignored_items WHERE host_id=? AND connector=? AND item=?`, hostID, connector, item)
	}
}

func (s *Store) queryResults(hostID string, ignored map[string][]string) []model.CheckResult {
	rows, err := s.db.Query(`
		SELECT connector,display_name,category,values_json,update_available,changelog,error,checked_at
		FROM results WHERE host_id=? ORDER BY connector
	`, hostID)
	if err != nil {
		return []model.CheckResult{}
	}
	defer rows.Close()

	var out []model.CheckResult
	for rows.Next() {
		var r model.CheckResult
		var valJSON string
		var upd int
		var checkedAt string
		if err := rows.Scan(&r.Connector, &r.DisplayName, &r.Category, &valJSON, &upd, &r.Changelog, &r.Error, &checkedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(valJSON), &r.Values)
		r.UpdateAvailable = upd == 1
		r.CheckedAt, _ = time.Parse(time.RFC3339Nano, checkedAt)

		if items, ok := ignored[r.Connector]; ok {
			connectorLevel := false
			var itemLevel []string
			for _, it := range items {
				if it == "" {
					connectorLevel = true
				} else {
					itemLevel = append(itemLevel, it)
				}
			}
			if connectorLevel {
				r.Ignored = true
				r.UpdateAvailable = false
			} else if len(itemLevel) > 0 {
				r.IgnoredItems = itemLevel
				r.Values = applyItemIgnores(r.Values, itemLevel, &r.UpdateAvailable)
			}
		}

		out = append(out, r)
	}
	return out
}

// applyItemIgnores removes ignored containers from the outdated/count values.
func applyItemIgnores(vals map[string]string, ignoredItems []string, updateAvailable *bool) map[string]string {
	outdated, ok := vals["outdated"]
	if !ok || outdated == "" {
		return vals
	}
	ignoredSet := map[string]bool{}
	for _, it := range ignoredItems {
		ignoredSet[strings.TrimSpace(it)] = true
	}
	containers := strings.Split(outdated, ",")
	var remaining []string
	for _, c := range containers {
		c = strings.TrimSpace(c)
		if c != "" && !ignoredSet[c] {
			remaining = append(remaining, c)
		}
	}
	// Copy map to avoid modifying shared state
	newVals := make(map[string]string, len(vals))
	for k, v := range vals {
		newVals[k] = v
	}
	if len(remaining) == 0 {
		*updateAvailable = false
		newVals["outdated"] = ""
		newVals["count"] = "0"
	} else {
		newVals["outdated"] = strings.Join(remaining, ",")
		newVals["count"] = strconv.Itoa(len(remaining))
	}
	return newVals
}

// ── Provisions ───────────────────────────────────────────────────────────────

func (s *Store) AddProvision(p model.Provision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connsJSON, _ := json.Marshal(p.Connectors)
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO provisions (token,name,host_type,connectors_json,created_at,claimed_by)
		VALUES (?,?,?,?,?,?)
	`, p.Token, p.Name, p.HostType, string(connsJSON), p.CreatedAt.UTC().Format(time.RFC3339Nano), p.ClaimedBy)
	if err != nil {
		log.Printf("store: add provision: %v", err)
	}
}

func (s *Store) GetProvision(token string) (*model.Provision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getProvision(token)
}

func (s *Store) getProvision(token string) (*model.Provision, bool) {
	var p model.Provision
	var connsJSON, createdAt string
	err := s.db.QueryRow(`SELECT token,name,host_type,connectors_json,created_at,claimed_by FROM provisions WHERE token=?`, token).
		Scan(&p.Token, &p.Name, &p.HostType, &connsJSON, &createdAt, &p.ClaimedBy)
	if err != nil {
		return nil, false
	}
	json.Unmarshal([]byte(connsJSON), &p.Connectors)
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &p, true
}

func (s *Store) ClaimProvision(token, hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`UPDATE provisions SET claimed_by=? WHERE token=?`, hostname, token)
}

func (s *Store) AllProvisions() []model.Provision {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT token,name,host_type,connectors_json,created_at,claimed_by FROM provisions ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Provision
	for rows.Next() {
		var p model.Provision
		var connsJSON, createdAt string
		if err := rows.Scan(&p.Token, &p.Name, &p.HostType, &connsJSON, &createdAt, &p.ClaimedBy); err != nil {
			continue
		}
		json.Unmarshal([]byte(connsJSON), &p.Connectors)
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, p)
	}
	return out
}

func (s *Store) DeleteProvision(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM provisions WHERE token=?`, token)
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (s *Store) AddCommand(c model.Command) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO commands (id,host_id,connector,cmd,status,output,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
	`, c.ID, c.HostID, c.Connector, c.Cmd, c.Status, c.Output,
		c.CreatedAt.UTC().Format(time.RFC3339Nano), c.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		log.Printf("store: add command: %v", err)
	}
}

func (s *Store) ClaimPending(hostID string) []model.Command {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id,connector,cmd FROM commands WHERE host_id=? AND status='pending'`, hostID)
	if err != nil {
		return nil
	}
	var cmds []model.Command
	for rows.Next() {
		var c model.Command
		rows.Scan(&c.ID, &c.Connector, &c.Cmd)
		cmds = append(cmds, c)
	}
	rows.Close()

	if len(cmds) > 0 {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, c := range cmds {
			s.db.Exec(`UPDATE commands SET status='running', updated_at=? WHERE id=?`, now, c.ID)
		}
	}
	return cmds
}

func (s *Store) FinishCommand(id, status, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`UPDATE commands SET status=?, output=?, updated_at=? WHERE id=?`,
		status, output, time.Now().UTC().Format(time.RFC3339Nano), id)
}

func (s *Store) HostCommands(hostID string) []model.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT id,host_id,connector,cmd,status,output,created_at,updated_at
		FROM commands WHERE host_id=? ORDER BY created_at DESC
	`, hostID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Command
	for rows.Next() {
		var c model.Command
		var ca, ua string
		rows.Scan(&c.ID, &c.HostID, &c.Connector, &c.Cmd, &c.Status, &c.Output, &ca, &ua)
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		out = append(out, c)
	}
	return out
}

func (s *Store) GetCommand(id string) (*model.Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c model.Command
	var ca, ua string
	err := s.db.QueryRow(`
		SELECT id,host_id,connector,cmd,status,output,created_at,updated_at FROM commands WHERE id=?
	`, id).Scan(&c.ID, &c.HostID, &c.Connector, &c.Cmd, &c.Status, &c.Output, &ca, &ua)
	if err != nil {
		return nil, false
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	return &c, true
}
