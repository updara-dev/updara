package security

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	kevURL       = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	nodeIndexURL = "https://nodejs.org/dist/index.json"
	eolURL       = "https://endoflife.date/api/%s.json"

	kevInterval   = time.Hour
	dailyInterval = 24 * time.Hour
)

// eolProducts maps inventory runtime names to endoflife.date product slugs.
var eolProducts = map[string]string{
	"node":    "nodejs",
	"python3": "python",
	"php":     "php",
}

type KEVEntry struct {
	CVE           string `json:"cveID"`
	VendorProject string `json:"vendorProject"`
	Product       string `json:"product"`
	Name          string `json:"vulnerabilityName"`
	DateAdded     string `json:"dateAdded"`
	Ransomware    string `json:"knownRansomwareCampaignUse"`
}

type NodeRelease struct {
	Version  string `json:"version"`
	Date     string `json:"date"`
	Security bool   `json:"security"`
}

type EOLCycle struct {
	Cycle  string          `json:"cycle"`
	EOL    json.RawMessage `json:"eol"` // date string or bool
	Latest string          `json:"latest"`
}

type FeedStatus struct {
	Name      string    `json:"name"`
	FetchedAt time.Time `json:"fetched_at"`
	Entries   int       `json:"entries"`
	Error     string    `json:"error,omitempty"`
}

// Feeds caches the upstream vulnerability data in memory. It is refetched on
// server start, so nothing is persisted.
type Feeds struct {
	mu     sync.RWMutex
	client *http.Client

	kev         []KEVEntry
	kevModified string
	node        []NodeRelease
	eol         map[string][]EOLCycle

	status    map[string]*FeedStatus
	attempted map[string]time.Time
}

func NewFeeds() *Feeds {
	return &Feeds{
		client: &http.Client{Timeout: 60 * time.Second},
		eol:    map[string][]EOLCycle{},
		status: map[string]*FeedStatus{
			"kev":       {Name: "CISA KEV"},
			"nodejs":    {Name: "Node.js releases"},
			"endoflife": {Name: "endoflife.date"},
		},
		attempted: map[string]time.Time{},
	}
}

// Refresh fetches every feed whose interval has elapsed (or all, if force).
// Returns true if any feed delivered new data.
func (f *Feeds) Refresh(force bool) bool {
	changed := false
	if force || f.due("kev", kevInterval) {
		changed = f.refreshKEV() || changed
	}
	if force || f.due("nodejs", dailyInterval) {
		changed = f.refreshNode() || changed
	}
	if force || f.due("endoflife", dailyInterval) {
		changed = f.refreshEOL() || changed
	}
	return changed
}

func (f *Feeds) due(name string, interval time.Duration) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.status[name].Error != "" {
		return time.Since(f.attempted[name]) > 15*time.Minute
	}
	return time.Since(f.status[name].FetchedAt) > interval
}

func (f *Feeds) setStatus(name string, entries int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempted[name] = time.Now()
	st := f.status[name]
	if err != nil {
		st.Error = err.Error()
		log.Printf("security: feed %s failed: %v", name, err)
		return
	}
	st.Error = ""
	st.FetchedAt = time.Now()
	if entries >= 0 {
		st.Entries = entries
	}
}

func (f *Feeds) get(url string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", "updara-security-radar")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

func (f *Feeds) refreshKEV() bool {
	f.mu.RLock()
	h := http.Header{}
	if f.kevModified != "" && len(f.kev) > 0 {
		h.Set("If-Modified-Since", f.kevModified)
	}
	f.mu.RUnlock()

	resp, err := f.get(kevURL, h)
	if err != nil {
		f.setStatus("kev", -1, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		f.setStatus("kev", -1, nil)
		return false
	}
	var doc struct {
		Vulnerabilities []KEVEntry `json:"vulnerabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		f.setStatus("kev", -1, err)
		return false
	}
	f.mu.Lock()
	f.kev = doc.Vulnerabilities
	f.kevModified = resp.Header.Get("Last-Modified")
	f.mu.Unlock()
	f.setStatus("kev", len(doc.Vulnerabilities), nil)
	return true
}

func (f *Feeds) refreshNode() bool {
	resp, err := f.get(nodeIndexURL, nil)
	if err != nil {
		f.setStatus("nodejs", -1, err)
		return false
	}
	defer resp.Body.Close()
	var rel []NodeRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		f.setStatus("nodejs", -1, err)
		return false
	}
	f.mu.Lock()
	f.node = rel
	f.mu.Unlock()
	f.setStatus("nodejs", len(rel), nil)
	return true
}

func (f *Feeds) refreshEOL() bool {
	fetched := map[string][]EOLCycle{}
	for _, product := range eolProducts {
		resp, err := f.get(fmt.Sprintf(eolURL, product), nil)
		if err != nil {
			f.setStatus("endoflife", -1, err)
			return false
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			f.setStatus("endoflife", -1, err)
			return false
		}
		var cycles []EOLCycle
		if err := json.Unmarshal(body, &cycles); err != nil {
			f.setStatus("endoflife", -1, fmt.Errorf("%s: %w", product, err))
			return false
		}
		fetched[product] = cycles
	}
	n := 0
	for _, c := range fetched {
		n += len(c)
	}
	f.mu.Lock()
	f.eol = fetched
	f.mu.Unlock()
	f.setStatus("endoflife", n, nil)
	return true
}

// Status returns a snapshot of all feeds, ordered for display.
func (f *Feeds) Status() []FeedStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]FeedStatus, 0, len(f.status))
	for _, k := range []string{"kev", "nodejs", "endoflife"} {
		out = append(out, *f.status[k])
	}
	return out
}

// Ready reports whether every feed has been loaded at least once.
func (f *Feeds) Ready() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.kev) > 0 && len(f.node) > 0 && len(f.eol) == len(eolProducts)
}
