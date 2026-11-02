package store

import (
	"encoding/json"
	"time"

	"github.com/updara/server/model"
)

// InventoryRuntimes returns the inventory connector's "runtimes" value per host.
func (s *Store) InventoryRuntimes() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT host_id, values_json FROM results WHERE connector='inventory' AND error=''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var hostID, valJSON string
		rows.Scan(&hostID, &valJSON)
		var vals map[string]string
		json.Unmarshal([]byte(valJSON), &vals)
		out[hostID] = vals["runtimes"]
	}
	return out
}

// HostDisplayNames maps host id → display name (falls back to hostname).
func (s *Store) HostDisplayNames() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, hostname, display_name FROM hosts`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, hostname, display string
		rows.Scan(&id, &hostname, &display)
		if display == "" {
			display = hostname
		}
		out[id] = display
	}
	return out
}

type openFinding struct {
	finding    model.SecurityFinding
	notifiedFP string
}

// SyncFindings stores a host's current findings. It returns the findings whose
// state differs from what was last notified, and previously notified findings
// that are gone now (patched).
func (s *Store) SyncFindings(hostID string, found []model.SecurityFinding) (pending, resolved []model.SecurityFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	open := map[string]openFinding{}
	rows, err := s.db.Query(`SELECT component, kind, data_json, fingerprint, notified_fp, first_seen
		FROM security_findings WHERE host_id=? AND resolved_at=''`, hostID)
	if err != nil {
		return nil, nil
	}
	for rows.Next() {
		var comp, kind, data, fp, nfp, first string
		rows.Scan(&comp, &kind, &data, &fp, &nfp, &first)
		var f model.SecurityFinding
		json.Unmarshal([]byte(data), &f)
		f.Fingerprint = fp
		f.FirstSeen, _ = time.Parse(time.RFC3339Nano, first)
		open[comp+"|"+kind] = openFinding{finding: f, notifiedFP: nfp}
	}
	rows.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	seen := map[string]bool{}
	for _, f := range found {
		key := f.Component() + "|" + f.Kind
		seen[key] = true
		data, _ := json.Marshal(f)
		if ex, ok := open[key]; ok {
			s.db.Exec(`UPDATE security_findings SET data_json=?, fingerprint=?
				WHERE host_id=? AND component=? AND kind=?`,
				string(data), f.Fingerprint, hostID, f.Component(), f.Kind)
			f.FirstSeen = ex.finding.FirstSeen
			f.Notified = ex.notifiedFP != ""
			if ex.notifiedFP != f.Fingerprint {
				pending = append(pending, f)
			}
			continue
		}
		s.db.Exec(`INSERT OR REPLACE INTO security_findings
			(host_id, component, kind, data_json, fingerprint, notified_fp, first_seen, resolved_at)
			VALUES (?,?,?,?,?,'',?,'')`,
			hostID, f.Component(), f.Kind, string(data), f.Fingerprint, nowStr)
		f.FirstSeen = now
		pending = append(pending, f)
	}

	for key, ex := range open {
		if seen[key] {
			continue
		}
		f := ex.finding
		s.db.Exec(`UPDATE security_findings SET resolved_at=? WHERE host_id=? AND component=? AND kind=?`,
			nowStr, hostID, f.Component(), f.Kind)
		if ex.notifiedFP != "" {
			f.Notified = true
			resolved = append(resolved, f)
		}
	}
	return pending, resolved
}

func (s *Store) MarkFindingsNotified(findings []model.SecurityFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range findings {
		s.db.Exec(`UPDATE security_findings SET notified_fp=? WHERE host_id=? AND component=? AND kind=?`,
			f.Fingerprint, f.HostID, f.Component(), f.Kind)
	}
}

// PruneFindings drops findings of hosts that no longer report an inventory.
func (s *Store) PruneFindings(activeHosts map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT DISTINCT host_id FROM security_findings`)
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		if !activeHosts[id] {
			stale = append(stale, id)
		}
	}
	rows.Close()
	for _, id := range stale {
		s.db.Exec(`DELETE FROM security_findings WHERE host_id=?`, id)
	}
}

// OpenFindings returns all unresolved findings across hosts.
func (s *Store) OpenFindings() []model.SecurityFinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT data_json, fingerprint, notified_fp, first_seen
		FROM security_findings WHERE resolved_at=''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []model.SecurityFinding{}
	for rows.Next() {
		var data, fp, nfp, first string
		rows.Scan(&data, &fp, &nfp, &first)
		var f model.SecurityFinding
		json.Unmarshal([]byte(data), &f)
		f.Fingerprint = fp
		f.Notified = nfp != ""
		f.FirstSeen, _ = time.Parse(time.RFC3339Nano, first)
		out = append(out, f)
	}
	return out
}
