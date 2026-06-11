package api

import (
	"encoding/json"
	"net/http"

	"github.com/updara/server/store"
)

// GET /api/v1/stats
func (h *Handler) globalStats(w http.ResponseWriter, r *http.Request) {
	stats := h.store.AllHostStats()
	if stats == nil {
		stats = []store.HostStatSummary{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GET /api/v1/hosts/{hostname}/stats
func (h *Handler) hostStats(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	history := h.store.HostUpdateHistory(hostname, 50)
	if history == nil {
		history = []store.UpdateRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}
