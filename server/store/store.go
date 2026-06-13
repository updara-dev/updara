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

CREATE TABLE IF NOT EXISTS disabled_connectors (
    host_id   TEXT NOT NULL,
    connector TEXT NOT NULL,
    PRIMARY KEY (host_id, connector)
);

CREATE TABLE IF NOT EXISTS deleted_hosts (
    host_id    TEXT PRIMARY KEY,
    deleted_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS notified_updates (
    host_id     TEXT NOT NULL,
    connector   TEXT NOT NULL,
    notified_at TEXT NOT NULL,
    PRIMARY KEY (host_id, connector)
);

CREATE TABLE IF NOT EXISTS notification_queue (
    hostname     TEXT NOT NULL,
    connector    TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    values_json  TEXT NOT NULL DEFAULT '{}',
    queued_at    TEXT NOT NULL,
    PRIMARY KEY (hostname, connector)
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
		`ALTER TABLE provisions ADD COLUMN server_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hosts ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE results ADD COLUMN update_since TEXT NOT NULL DEFAULT ''`,
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

	var deleted int
	s.db.QueryRow(`SELECT COUNT(*) FROM deleted_hosts WHERE host_id=?`, req.Hostname).Scan(&deleted)
	if deleted > 0 {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO hosts (id, hostname, ip_address, agent_version, last_seen, display_name) VALUES (?,?,?,?,?,'')
		ON CONFLICT(id) DO UPDATE SET
			ip_address=excluded.ip_address,
			agent_version=excluded.agent_version,
			last_seen=excluded.last_seen
	`, req.Hostname, req.Hostname, req.IPAddress, req.AgentVersion, now)
	if err != nil {
		log.Printf("store: upsert host: %v", err)
		return
	}

	disabled := s.fetchDisabled(req.Hostname)

	// Preserve update_since timestamps for connectors that remain outdated
	sinceMap := map[string]string{}
	rows, _ := s.db.Query(
		`SELECT connector, update_since FROM results WHERE host_id=? AND update_available=1 AND update_since!=''`,
		req.Hostname)
	if rows != nil {
		for rows.Next() {
			var conn, since string
			rows.Scan(&conn, &since)
			sinceMap[conn] = since
		}
		rows.Close()
	}

	s.db.Exec(`DELETE FROM results WHERE host_id = ?`, req.Hostname)
	for _, r := range req.Results {
		if disabled[r.Connector] {
			continue
		}
		valJSON, _ := json.Marshal(r.Values)
		upd := 0
		if r.UpdateAvailable {
			upd = 1
		}
		since := ""
		if upd == 1 {
			since = sinceMap[r.Connector]
			if since == "" {
				since = now
			}
		}
		s.db.Exec(`
			INSERT INTO results
			  (host_id,connector,display_name,category,values_json,update_available,changelog,error,checked_at,update_since)
			VALUES (?,?,?,?,?,?,?,?,?,?)
		`, req.Hostname, r.Connector, r.DisplayName, r.Category, string(valJSON),
			upd, r.Changelog, r.Error, r.CheckedAt.UTC().Format(time.RFC3339Nano), since)
	}
}

func (s *Store) RenameHost(hostname, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE hosts SET display_name=? WHERE id=?`, displayName, hostname)
	return err
}

func (s *Store) IsDeleted(hostname string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM deleted_hosts WHERE host_id=?`, hostname).Scan(&count)
	return count > 0
}

func (s *Store) fetchDisabled(hostID string) map[string]bool {
	rows, err := s.db.Query(`SELECT connector FROM disabled_connectors WHERE host_id=?`, hostID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out[c] = true
	}
	return out
}

func (s *Store) AllHosts() []model.HostStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT id, hostname, display_name, ip_address, agent_version, last_seen FROM hosts ORDER BY hostname`)
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
		if err := rows.Scan(&h.ID, &h.Hostname, &h.DisplayName, &h.IPAddress, &h.AgentVersion, &lastSeen); err != nil {
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
	err := s.db.QueryRow(`SELECT id, hostname, display_name, ip_address, agent_version, last_seen FROM hosts WHERE id=?`, hostname).
		Scan(&h.ID, &h.Hostname, &h.DisplayName, &h.IPAddress, &h.AgentVersion, &lastSeen)
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
		INSERT OR REPLACE INTO provisions (token,name,host_type,connectors_json,created_at,claimed_by,server_url)
		VALUES (?,?,?,?,?,?,?)
	`, p.Token, p.Name, p.HostType, string(connsJSON), p.CreatedAt.UTC().Format(time.RFC3339Nano), p.ClaimedBy, p.ServerURL)
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
	err := s.db.QueryRow(`SELECT token,name,host_type,connectors_json,created_at,claimed_by,server_url FROM provisions WHERE token=?`, token).
		Scan(&p.Token, &p.Name, &p.HostType, &connsJSON, &createdAt, &p.ClaimedBy, &p.ServerURL)
	if err != nil {
		return nil, false
	}
	json.Unmarshal([]byte(connsJSON), &p.Connectors)
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &p, true
}

func (s *Store) ProvisionByHost(hostname string) (*model.Provision, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p model.Provision
	var connsJSON, createdAt string
	err := s.db.QueryRow(`SELECT token,name,host_type,connectors_json,created_at,claimed_by,server_url FROM provisions WHERE claimed_by=? ORDER BY created_at DESC LIMIT 1`, hostname).
		Scan(&p.Token, &p.Name, &p.HostType, &connsJSON, &createdAt, &p.ClaimedBy, &p.ServerURL)
	if err != nil {
		return nil, false
	}
	json.Unmarshal([]byte(connsJSON), &p.Connectors)
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &p, true
}

// ClaimProvision marks the provision as claimed and returns the provision's
// canonical hostname (provision.name). This is used to override the hostname
// an agent reports when its systemd env var got truncated due to spaces.
func (s *Store) ClaimProvision(token, hostname string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var canonical string
	s.db.QueryRow(`SELECT name FROM provisions WHERE token=?`, token).Scan(&canonical)
	s.db.Exec(`UPDATE provisions SET claimed_by=? WHERE token=?`, hostname, token)
	if canonical != "" {
		s.db.Exec(`DELETE FROM deleted_hosts WHERE host_id=?`, canonical)
	} else {
		s.db.Exec(`DELETE FROM deleted_hosts WHERE host_id=?`, hostname)
	}
	return canonical
}

func (s *Store) AllProvisions() []model.Provision {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT token,name,host_type,connectors_json,created_at,claimed_by,server_url FROM provisions ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.Provision
	for rows.Next() {
		var p model.Provision
		var connsJSON, createdAt string
		if err := rows.Scan(&p.Token, &p.Name, &p.HostType, &connsJSON, &createdAt, &p.ClaimedBy, &p.ServerURL); err != nil {
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

func (s *Store) DeleteHostConnector(hostID, connector string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`INSERT OR REPLACE INTO disabled_connectors (host_id, connector) VALUES (?,?)`, hostID, connector)
	s.db.Exec(`DELETE FROM results WHERE host_id=? AND connector=?`, hostID, connector)
	s.db.Exec(`DELETE FROM ignored_items WHERE host_id=? AND connector=?`, hostID, connector)
}

func (s *Store) EnableConnector(hostID, connector string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM disabled_connectors WHERE host_id=? AND connector=?`, hostID, connector)
}

func (s *Store) DeleteHost(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.db.Exec(`INSERT OR REPLACE INTO deleted_hosts (host_id, deleted_at) VALUES (?,?)`, hostname, now)
	s.db.Exec(`DELETE FROM hosts WHERE id=?`, hostname)
	s.db.Exec(`DELETE FROM results WHERE host_id=?`, hostname)
	s.db.Exec(`DELETE FROM ignored_items WHERE host_id=?`, hostname)
	s.db.Exec(`DELETE FROM disabled_connectors WHERE host_id=?`, hostname)
	s.db.Exec(`DELETE FROM commands WHERE host_id=? AND status='pending'`, hostname)
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

// ── Settings ─────────────────────────────────────────────────────────────────

func (s *Store) GetSetting(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v
}

func (s *Store) SetSetting(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
}

func (s *Store) AllSettings() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		out[k] = v
	}
	return out
}

// ── Notification dedup ────────────────────────────────────────────────────────

type UpdateEntry struct {
	Connector, DisplayName, ValuesJSON string
}

// NewUpdates returns connectors that are update_available=true for hostname
// and either never notified, or last notified more than cooldownDays ago.
func (s *Store) NewUpdates(hostname string, cooldownDays int) []UpdateEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := `
		SELECT r.connector, r.display_name, COALESCE(r.values_json,'{}')
		FROM results r
		WHERE r.host_id=? AND r.update_available=1
		  AND NOT EXISTS (
		      SELECT 1 FROM ignored_items i
		      WHERE i.host_id=r.host_id AND i.connector=r.connector AND i.item=''
		  )
		  AND (
		    NOT EXISTS (
		        SELECT 1 FROM notified_updates n
		        WHERE n.host_id=r.host_id AND n.connector=r.connector
		    )
		    OR EXISTS (
		        SELECT 1 FROM notified_updates n
		        WHERE n.host_id=r.host_id AND n.connector=r.connector
		          AND datetime(n.notified_at) < datetime('now', '-` + strconv.Itoa(cooldownDays) + ` days')
		    )
		  )
	`
	rows, err := s.db.Query(q, hostname)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []UpdateEntry
	for rows.Next() {
		var e UpdateEntry
		rows.Scan(&e.Connector, &e.DisplayName, &e.ValuesJSON)
		out = append(out, e)
	}
	return out
}

// ParseCount extracts the "count" field from a values_json string, returns -1 if absent.
func ParseCount(valuesJSON string) int {
	var m map[string]string
	if json.Unmarshal([]byte(valuesJSON), &m) == nil {
		if s, ok := m["count"]; ok {
			if n, err := strconv.Atoi(s); err == nil {
				return n
			}
		}
	}
	return -1
}

// MarkNotified records that we notified about these connectors for hostname.
func (s *Store) MarkNotified(hostname string, entries []UpdateEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, e := range entries {
		s.db.Exec(`INSERT OR REPLACE INTO notified_updates(host_id,connector,notified_at) VALUES(?,?,?)`,
			hostname, e.Connector, now)
	}
}

// ClearResolved removes notified_updates entries where update is no longer available,
// so the next update cycle triggers a fresh notification.
func (s *Store) ClearResolved(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`
		DELETE FROM notified_updates
		WHERE host_id=?
		  AND NOT EXISTS (
		      SELECT 1 FROM results r
		      WHERE r.host_id=notified_updates.host_id
		        AND r.connector=notified_updates.connector
		        AND r.update_available=1
		  )
		  AND EXISTS (
		      SELECT 1 FROM results r
		      WHERE r.host_id=notified_updates.host_id
		        AND r.connector=notified_updates.connector
		        AND (r.error IS NULL OR r.error='')
		  )
	`, hostname)
}

// HostStatSummary is the per-host row returned by AllHostStats.
type HostStatSummary struct {
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ip_address"`
	LastUpdate   string `json:"last_update"`   // RFC3339 or ""
	TotalDone    int    `json:"total_done"`
	Done30Days   int    `json:"done_30days"`
	TopConnector string `json:"top_connector"` // most-updated connector, "" if none
}

// UpdateRecord is one entry in a host's update history.
type UpdateRecord struct {
	Connector   string `json:"connector"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// AllHostStats returns a summary row for every known host.
func (s *Store) AllHostStats() []HostStatSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT h.hostname, h.ip_address,
		       COALESCE(MAX(CASE WHEN c.status='done' THEN c.updated_at END), '') as last_update,
		       SUM(CASE WHEN c.status='done' THEN 1 ELSE 0 END) as total_done,
		       SUM(CASE WHEN c.status='done' AND c.updated_at >= datetime('now','-30 days') THEN 1 ELSE 0 END) as done_30d
		FROM hosts h
		LEFT JOIN commands c ON c.host_id=h.id AND c.connector NOT GLOB '__*'
		GROUP BY h.id, h.hostname, h.ip_address
		ORDER BY last_update DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []HostStatSummary
	for rows.Next() {
		var r HostStatSummary
		rows.Scan(&r.Hostname, &r.IPAddress, &r.LastUpdate, &r.TotalDone, &r.Done30Days)
		out = append(out, r)
	}
	rows.Close()
	// Fetch top connector per host (separate query to keep above query simple)
	for i, r := range out {
		var top string
		s.db.QueryRow(`
			SELECT connector FROM commands
			WHERE host_id=(SELECT id FROM hosts WHERE hostname=?)
			  AND connector NOT GLOB '__*' AND status='done'
			GROUP BY connector ORDER BY COUNT(*) DESC LIMIT 1
		`, r.Hostname).Scan(&top)
		out[i].TopConnector = top
	}
	return out
}

// HostUpdateHistory returns the last N update commands for a host (all statuses).
func (s *Store) HostUpdateHistory(hostname string, limit int) []UpdateRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT c.connector,
		       COALESCE(r.display_name, c.connector),
		       c.status, c.created_at, c.updated_at
		FROM commands c
		LEFT JOIN results r ON r.host_id=c.host_id AND r.connector=c.connector
		WHERE c.host_id=(SELECT id FROM hosts WHERE hostname=?)
		  AND c.connector NOT GLOB '__*'
		ORDER BY c.updated_at DESC
		LIMIT ?
	`, hostname, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []UpdateRecord
	for rows.Next() {
		var rec UpdateRecord
		rows.Scan(&rec.Connector, &rec.DisplayName, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
		out = append(out, rec)
	}
	return out
}

// EnqueueNotifications adds entries to the notification queue for batched sending.
func (s *Store) EnqueueNotifications(hostname string, entries []UpdateEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, e := range entries {
		s.db.Exec(`INSERT OR REPLACE INTO notification_queue(hostname,connector,display_name,values_json,queued_at) VALUES(?,?,?,?,?)`,
			hostname, e.Connector, e.DisplayName, e.ValuesJSON, now)
	}
}

// FlushQueue returns all queued notifications grouped by hostname, then clears the queue.
// Only entries that are still update_available and not ignored are returned.
func (s *Store) FlushQueue() map[string][]UpdateEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT q.hostname, q.connector, q.display_name, q.values_json
		FROM notification_queue q
		JOIN hosts h ON h.hostname = q.hostname
		JOIN results r ON r.host_id = h.id AND r.connector = q.connector
		WHERE r.update_available = 1
		  AND NOT EXISTS (
		      SELECT 1 FROM ignored_items i
		      WHERE i.host_id = h.id AND i.connector = q.connector AND i.item = ''
		  )
		ORDER BY q.hostname, q.queued_at
	`)
	if err != nil {
		return nil
	}
	result := make(map[string][]UpdateEntry)
	for rows.Next() {
		var hostname string
		var e UpdateEntry
		rows.Scan(&hostname, &e.Connector, &e.DisplayName, &e.ValuesJSON)
		result[hostname] = append(result[hostname], e)
	}
	rows.Close()
	s.db.Exec(`DELETE FROM notification_queue`)
	return result
}

type DigestUpdate struct {
	Name  string
	Since time.Time // zero if unknown
}

// DigestHost holds per-host summary data for the digest email.
type DigestHost struct {
	Hostname    string
	DisplayName string
	IPAddress   string
	LastSeen    time.Time
	Updates     []DigestUpdate
	Errors      []string
}

func (s *Store) DigestSummary() []DigestHost {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT h.hostname, h.display_name, h.ip_address, h.last_seen,
		       r.display_name, r.update_available, r.error, r.ignored, r.update_since
		FROM hosts h
		LEFT JOIN (
			SELECT r2.host_id, r2.connector, r2.display_name,
			       r2.update_available, r2.error, r2.update_since,
			       CASE WHEN ig.host_id IS NOT NULL THEN 1 ELSE 0 END as ignored
			FROM results r2
			LEFT JOIN ignored_items ig ON ig.host_id = r2.host_id
			    AND ig.connector = r2.connector AND ig.item = ''
		) r ON r.host_id = h.id
		ORDER BY h.hostname, r.connector
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	hostMap := map[string]*DigestHost{}
	var order []string
	for rows.Next() {
		var hostname, displayName, ip, lastSeen string
		var connName, errStr, updateSince sql.NullString
		var updateAvail, ignored sql.NullInt64
		if err := rows.Scan(&hostname, &displayName, &ip, &lastSeen,
			&connName, &updateAvail, &errStr, &ignored, &updateSince); err != nil {
			continue
		}
		if _, ok := hostMap[hostname]; !ok {
			t, _ := time.Parse(time.RFC3339Nano, lastSeen)
			hostMap[hostname] = &DigestHost{
				Hostname: hostname, DisplayName: displayName,
				IPAddress: ip, LastSeen: t,
			}
			order = append(order, hostname)
		}
		h := hostMap[hostname]
		if !connName.Valid || ignored.Int64 == 1 {
			continue
		}
		name := connName.String
		if updateAvail.Valid && updateAvail.Int64 == 1 {
			var since time.Time
			if updateSince.Valid && updateSince.String != "" {
				since, _ = time.Parse(time.RFC3339Nano, updateSince.String)
			}
			h.Updates = append(h.Updates, DigestUpdate{Name: name, Since: since})
		}
		if errStr.Valid && errStr.String != "" {
			h.Errors = append(h.Errors, name)
		}
	}

	out := make([]DigestHost, 0, len(order))
	for _, hn := range order {
		out = append(out, *hostMap[hn])
	}
	return out
}
