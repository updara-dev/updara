package store

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/updara/server/model"
)

type Store struct {
	mu         sync.RWMutex
	hosts      map[string]*model.Host
	results    map[string][]model.CheckResult
	provisions map[string]*model.Provision
	path       string
}

type disk struct {
	Hosts      map[string]*model.Host         `json:"hosts"`
	Results    map[string][]model.CheckResult `json:"results"`
	Provisions map[string]*model.Provision    `json:"provisions"`
}

func New(path string) *Store {
	s := &Store{
		hosts:      make(map[string]*model.Host),
		results:    make(map[string][]model.CheckResult),
		provisions: make(map[string]*model.Provision),
		path:       path,
	}
	s.load()
	return s
}

func (s *Store) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var d disk
	if err := json.Unmarshal(data, &d); err != nil {
		log.Printf("store: load error: %v", err)
		return
	}
	if d.Hosts != nil {
		s.hosts = d.Hosts
	}
	if d.Results != nil {
		s.results = d.Results
	}
	if d.Provisions != nil {
		s.provisions = d.Provisions
	}
	log.Printf("store: loaded %d host(s), %d provision(s)", len(s.hosts), len(s.provisions))
}

func (s *Store) save() {
	if s.path == "" {
		return
	}
	data, err := json.Marshal(disk{
		Hosts:      s.hosts,
		Results:    s.results,
		Provisions: s.provisions,
	})
	if err != nil {
		log.Printf("store: marshal error: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("store: write error: %v", err)
		return
	}
	os.Rename(tmp, s.path)
}

// ── Hosts ────────────────────────────────────────────────────────────────────

func (s *Store) Upsert(req model.ReportRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.Hostname
	s.hosts[id] = &model.Host{
		ID:           id,
		Hostname:     req.Hostname,
		AgentVersion: req.AgentVersion,
		LastSeen:     time.Now(),
	}
	s.results[id] = req.Results
	s.save()
}

func (s *Store) AllHosts() []model.HostStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.HostStatus, 0, len(s.hosts))
	for id, h := range s.hosts {
		results := s.results[id]
		if results == nil {
			results = []model.CheckResult{}
		}
		out = append(out, model.HostStatus{Host: *h, Results: results})
	}
	return out
}

// ── Provisions ───────────────────────────────────────────────────────────────

func (s *Store) AddProvision(p model.Provision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provisions[p.Token] = &p
	s.save()
}

func (s *Store) GetProvision(token string) (*model.Provision, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.provisions[token]
	return p, ok
}

func (s *Store) ClaimProvision(token, hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.provisions[token]; ok {
		p.ClaimedBy = hostname
	}
	s.save()
}

func (s *Store) AllProvisions() []model.Provision {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Provision, 0, len(s.provisions))
	for _, p := range s.provisions {
		out = append(out, *p)
	}
	return out
}

func (s *Store) DeleteProvision(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.provisions, token)
	s.save()
}
