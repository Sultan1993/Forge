package api

import (
	"net/http"
	"os/exec"
	"strings"
)

// Common install locations that may not be in root's PATH.
var searchPaths = []string{
	"/opt/homebrew/bin",     // macOS Homebrew (Apple Silicon)
	"/usr/local/bin",        // macOS Homebrew (Intel) / manual installs
	"/usr/local/go/bin",     // Go official installer
	"/usr/bin",              // system
	"/bin",                  // system
	"/snap/bin",             // Linux snap
}

type toolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Found   bool   `json:"found"`
}

func handleEnvironment(w http.ResponseWriter, r *http.Request) {
	tools := []struct {
		name  string
		bins  []string // binary names to look for
		args  []string // version flag(s)
	}{
		{"Node.js", []string{"node"}, []string{"--version"}},
		{"Python", []string{"python3", "python"}, []string{"--version"}},
		{"Go", []string{"go"}, []string{"version"}},
		{"Rust", []string{"rustc"}, []string{"--version"}},
		{"Java", []string{"java"}, []string{"--version"}},
		{"Ruby", []string{"ruby"}, []string{"--version"}},
		{"Docker", []string{"docker"}, []string{"--version"}},
		{"Git", []string{"git"}, []string{"--version"}},
		{"Homebrew", []string{"brew"}, []string{"--version"}},
		{"npm", []string{"npm"}, []string{"--version"}},
		{"pnpm", []string{"pnpm"}, []string{"--version"}},
	}

	var result []toolInfo
	for _, t := range tools {
		info := toolInfo{Name: t.name}
		for _, bin := range t.bins {
			if path := findBinary(bin); path != "" {
				out, err := exec.Command(path, t.args...).CombinedOutput()
				if err == nil {
					info.Found = true
					info.Version = cleanVersion(string(out))
					break
				}
			}
		}
		result = append(result, info)
	}

	writeJSON(w, http.StatusOK, result)
}

// findBinary searches for a binary in PATH and common install locations.
func findBinary(name string) string {
	// Check PATH first
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	// Check common locations
	for _, dir := range searchPaths {
		p := dir + "/" + name
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return ""
}

func cleanVersion(s string) string {
	s = strings.TrimSpace(s)
	// Take first line only
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	// Remove common prefixes
	for _, prefix := range []string{"v", "go version ", "Python ", "ruby ", "java ", "git version ", "Docker version ", "Homebrew "} {
		s = strings.TrimPrefix(s, prefix)
	}
	// Trim trailing comma or extra info after comma
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
