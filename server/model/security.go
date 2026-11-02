package model

import "time"

// SecurityFinding is one vulnerable runtime on one host.
// Kind: "security" (newer security release exists), "kev" (possibly affected
// by an actively exploited CVE), "eol" (runtime no longer receives fixes).
type SecurityFinding struct {
	HostID      string    `json:"host_id"`
	HostName    string    `json:"host_name"`
	Location    string    `json:"location"` // "host" or container name
	Runtime     string    `json:"runtime"`  // node | python3 | php
	Kind        string    `json:"kind"`
	Installed   string    `json:"installed"`
	Fixed       string    `json:"fixed"`
	CVEs        []string  `json:"cves"`
	KEV         bool      `json:"kev"`
	Distro      bool      `json:"distro"`
	Detail      string    `json:"detail"`
	FirstSeen   time.Time `json:"first_seen"`
	Notified    bool      `json:"notified"`
	Fingerprint string    `json:"-"`
}

func (f SecurityFinding) Component() string {
	return f.Runtime + "@" + f.Location
}
