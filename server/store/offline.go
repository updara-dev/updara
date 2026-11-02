package store

import (
	"time"
)

// AgentState is the offline-watch view of one host.
type AgentState struct {
	ID         string
	Name       string
	LastSeen   time.Time
	Public     bool
	MutedUntil string
	NotifiedAt string
}

// Muted reports whether the offline alert is silenced at now.
func (a AgentState) Muted(now time.Time) bool {
	if a.MutedUntil == "" {
		return false
	}
	if a.MutedUntil == "forever" {
		return true
	}
	t, err := time.Parse(time.RFC3339, a.MutedUntil)
	return err == nil && now.Before(t)
}

func (s *Store) AgentStates() []AgentState {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, display_name, last_seen, public, offline_muted_until, offline_notified_at FROM hosts ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AgentState
	for rows.Next() {
		var a AgentState
		var display, lastSeen string
		if rows.Scan(&a.ID, &display, &lastSeen, &a.Public, &a.MutedUntil, &a.NotifiedAt) != nil {
			continue
		}
		a.Name = display
		if a.Name == "" {
			a.Name = a.ID
		}
		a.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
		out = append(out, a)
	}
	return out
}

func (s *Store) SetOfflineNotified(hostID string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := ""
	if !at.IsZero() {
		v = at.UTC().Format(time.RFC3339)
	}
	s.db.Exec(`UPDATE hosts SET offline_notified_at=? WHERE id=?`, v, hostID)
}

func (s *Store) SetHostMonitoring(hostID string, public bool, mutedUntil string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE hosts SET public=?, offline_muted_until=? WHERE id=?`, public, mutedUntil, hostID)
	return err
}

// PublicHosts returns the set of hosts flagged as publicly reachable.
func (s *Store) PublicHosts() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM hosts WHERE public=1`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out[id] = true
	}
	return out
}
