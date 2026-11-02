package security

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/updara/server/model"
)

type Component struct {
	Runtime  string
	Version  string
	Location string
	Distro   bool
}

// ParseRuntimes parses the inventory connector's "runtimes" value:
// "node 24.16.0@host, python3 3.11.2~distro@host, node 22.23.2@npm".
func ParseRuntimes(s string) []Component {
	var out []Component
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		at := strings.LastIndex(item, "@")
		if at < 0 {
			continue
		}
		parts := strings.Fields(item[:at])
		if len(parts) != 2 {
			continue
		}
		v := parts[1]
		distro := strings.HasSuffix(v, "~distro")
		out = append(out, Component{
			Runtime:  parts[0],
			Version:  strings.TrimSuffix(v, "~distro"),
			Location: item[at+1:],
			Distro:   distro,
		})
	}
	return out
}

// CISA lists vulnerabilities in libraries Node bundles under the library's
// own name (Node2Shell = "Google Brotli"), so those count for Node too.
var kevProducts = map[string][]string{
	"node":    {"node.js", "nodejs", "brotli", "openssl", "zlib", "libuv", "llhttp", "undici", "c-ares", "nghttp2"},
	"python3": {"python"},
	"php":     {"php"},
}

// Match returns the findings for one host's components. Host fields are left
// empty for the caller to fill in.
func (f *Feeds) Match(comps []Component, now time.Time) []model.SecurityFinding {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var out []model.SecurityFinding
	for _, c := range comps {
		base := model.SecurityFinding{
			Location:  c.Location,
			Runtime:   c.Runtime,
			Installed: c.Version,
			Distro:    c.Distro,
		}

		cy := f.cycle(c.Runtime, c.Version)
		eol, eolDate := false, ""
		if cy != nil {
			eol, eolDate = isEOL(cy.EOL, now)
		}

		if eol {
			fd := base
			fd.Kind = "eol"
			fd.Detail = "end of life — no more security fixes"
			since := now.AddDate(-1, 0, 0)
			if t, err := time.Parse("2006-01-02", eolDate); err == nil {
				since = t
				fd.Detail = "end of life since " + eolDate + " — no more security fixes"
			}
			// Exploited after EOL means no fix will ever ship for this line.
			if kev := f.matchKEV(&fd, kevProducts[c.Runtime], since, time.Time{}); kev != "" {
				fd.Detail += "; actively exploited, will not be fixed (CISA KEV): " + kev
			}
			out = append(out, fd)
		}

		if c.Runtime == "node" {
			if fix, fixDate, relDate := f.nodeSecurityFix(c.Version); fix != "" {
				fd := base
				fd.Kind = "security"
				fd.Fixed = fix
				fd.Detail = "Node.js security release " + fix + " available"
				// KEV only counts if it lands between the installed release and
				// shortly after the fix — later entries are not fixed by it.
				since := now.AddDate(0, 0, -90)
				if t, err := time.Parse("2006-01-02", relDate); err == nil {
					since = t
				}
				var until time.Time
				if t, err := time.Parse("2006-01-02", fixDate); err == nil {
					until = t.AddDate(0, 0, 60)
				}
				if kev := f.matchKEV(&fd, kevProducts["node"], since, until); kev != "" {
					fd.Detail += " — actively exploited (CISA KEV): " + kev
				}
				out = append(out, fd)
			}
			continue
		}

		if !eol && cy != nil && cmpVersion(cy.Latest, c.Version) > 0 {
			fd := base
			fd.Kind = "kev"
			fd.Fixed = cy.Latest
			if kev := f.matchKEV(&fd, kevProducts[c.Runtime], now.AddDate(-1, 0, 0), time.Time{}); kev != "" {
				fd.Detail = "possibly affected, not on the latest patch release — actively exploited (CISA KEV): " + kev
				out = append(out, fd)
			}
		}
	}

	for i := range out {
		out[i].Fingerprint = fingerprint(out[i])
	}
	return out
}

func (f *Feeds) cycle(runtime, version string) *EOLCycle {
	parts := versionParts(version)
	if len(parts) == 0 {
		return nil
	}
	name := strconv.Itoa(parts[0])
	if runtime != "node" && len(parts) > 1 {
		name += "." + strconv.Itoa(parts[1])
	}
	for i, cy := range f.eol[eolProducts[runtime]] {
		if cy.Cycle == name {
			return &f.eol[eolProducts[runtime]][i]
		}
	}
	return nil
}

// nodeSecurityFix returns the newest security release in the installed major
// line that is newer than installed, its date, and the installed release date.
func (f *Feeds) nodeSecurityFix(installed string) (fix, fixDate, relDate string) {
	major := versionParts(installed)
	if len(major) == 0 {
		return "", "", ""
	}
	for _, r := range f.node {
		v := strings.TrimPrefix(r.Version, "v")
		if cmpVersion(v, installed) == 0 {
			relDate = r.Date
		}
		p := versionParts(v)
		if !r.Security || len(p) == 0 || p[0] != major[0] || cmpVersion(v, installed) <= 0 {
			continue
		}
		if fix == "" || cmpVersion(v, fix) > 0 {
			fix, fixDate = v, r.Date
		}
	}
	return fix, fixDate, relDate
}

// matchKEV adds KEV entries for the given products added within [since, until]
// (zero until = open-ended) to fd and returns them as a readable list.
func (f *Feeds) matchKEV(fd *model.SecurityFinding, products []string, since, until time.Time) string {
	var names []string
	for _, k := range f.kev {
		added, err := time.Parse("2006-01-02", k.DateAdded)
		if err != nil || added.Before(since.Truncate(24*time.Hour)) || (!until.IsZero() && added.After(until)) {
			continue
		}
		p := strings.ToLower(k.Product)
		for _, want := range products {
			if p == want {
				fd.CVEs = append(fd.CVEs, k.CVE)
				names = append(names, fmt.Sprintf("%s (%s %s)", k.CVE, k.VendorProject, k.Product))
				break
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	fd.KEV = true
	sort.Strings(fd.CVEs)
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func isEOL(raw json.RawMessage, now time.Time) (bool, string) {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return false, ""
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false, ""
	}
	return now.After(t), s
}

func fingerprint(f model.SecurityFinding) string {
	return fmt.Sprintf("%s|%s|%s|%t", f.Kind, f.Fixed, strings.Join(f.CVEs, ","), f.KEV)
}

// versionParts parses "24.16.0", "v3.12.3", "8.3.12-1" into numeric parts.
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+~ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

func cmpVersion(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
