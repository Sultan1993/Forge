package tailscale

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
)

type Device struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Online bool   `json:"online"`
	IsSelf bool   `json:"isSelf"`
	OS     string `json:"os"`
}

type Status struct {
	Self    Device   `json:"self"`
	Devices []Device `json:"devices"`
}

// tailscaleStatus is the relevant subset of `tailscale status --json` output.
type tailscaleStatus struct {
	Self *tsPeer            `json:"Self"`
	Peer map[string]*tsPeer `json:"Peer"`
}

type tsPeer struct {
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	OS           string   `json:"OS"`
}

// findTailscale returns the path to the tailscale binary.
func findTailscale() string {
	// Check common locations — launchd/systemd run with a minimal PATH
	// that often doesn't include Homebrew or snap paths.
	paths := []string{
		"tailscale",                    // in PATH
		"/opt/homebrew/bin/tailscale",  // macOS Homebrew (Apple Silicon)
		"/usr/local/bin/tailscale",     // macOS Homebrew (Intel) / Linux manual
		"/usr/bin/tailscale",           // Linux package manager
		"/snap/bin/tailscale",          // Linux snap
	}
	for _, p := range paths {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	return "tailscale"
}

// GetStatus calls `tailscale status --json` and returns a simplified device list.
func GetStatus() (*Status, error) {
	out, err := exec.Command(findTailscale(), "status", "--json").Output()
	if err != nil {
		return nil, err
	}

	var raw tailscaleStatus
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}

	status := &Status{}

	if raw.Self != nil {
		status.Self = peerToDevice(raw.Self, true)
	}

	// Self always first in the list
	status.Devices = append(status.Devices, status.Self)

	// Collect peers, skip nil entries
	for _, peer := range raw.Peer {
		if peer == nil {
			continue
		}
		status.Devices = append(status.Devices, peerToDevice(peer, false))
	}

	// Sort peers (after self) by name for stable UI ordering
	if len(status.Devices) > 1 {
		peers := status.Devices[1:]
		sort.Slice(peers, func(i, j int) bool {
			return peers[i].Name < peers[j].Name
		})
	}

	return status, nil
}

func peerToDevice(p *tsPeer, isSelf bool) Device {
	d := Device{
		Name:   cleanDNSName(p.DNSName),
		Online: p.Online || isSelf, // Self is always online
		IsSelf: isSelf,
		OS:     p.OS,
	}
	if len(p.TailscaleIPs) > 0 {
		d.IP = p.TailscaleIPs[0]
	}
	return d
}

func cleanDNSName(name string) string {
	// DNSName looks like "mac-studio.tailnet-name.ts.net."
	// We want just the hostname part.
	name = strings.TrimSuffix(name, ".")
	parts := strings.SplitN(name, ".", 2)
	return parts[0]
}
