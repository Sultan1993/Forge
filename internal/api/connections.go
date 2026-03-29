package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Sultan1993/forge/internal/system"
	"github.com/Sultan1993/forge/internal/tailscale"
)

type tmuxSession struct {
	Name     string `json:"name"`
	Windows  string `json:"windows"`
	Created  string `json:"created"`
	Attached bool   `json:"attached"`
}

func getTmuxSessions() []tmuxSession {
	// Find the console user to run tmux as
	userOut, err := exec.Command("stat", "-f", "%Su", "/dev/console").Output()
	if err != nil {
		return nil
	}
	user := strings.TrimSpace(string(userOut))
	if user == "" || user == "root" {
		return nil
	}

	// Run tmux list-sessions as the console user
	out, err := exec.Command("su", "-", user, "-c",
		`tmux list-sessions -F '#{session_name}|#{session_windows}|#{session_created}|#{session_attached}' 2>/dev/null`,
	).Output()
	if err != nil {
		return nil
	}

	var sessions []tmuxSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		// Parse creation timestamp
		created := parts[2]
		if ts, err := time.Parse("1136239445", parts[2]); err == nil {
			// Unix timestamp as string
			created = ts.Format(time.RFC3339)
		} else {
			// Try parsing as unix epoch integer
			var epoch int64
			if _, err := fmt.Sscanf(parts[2], "%d", &epoch); err == nil {
				created = time.Unix(epoch, 0).Format(time.RFC3339)
			}
		}

		sessions = append(sessions, tmuxSession{
			Name:     parts[0],
			Windows:  parts[1] + " window" + (map[bool]string{true: "s", false: ""}[parts[1] != "1"]),
			Created:  created,
			Attached: parts[3] != "0",
		})
	}
	return sessions
}

var (
	sessionMu        sync.Mutex
	lastSessionKeys  = map[string]bool{}
	firstSessionPoll = true
)

func handleGetConnections(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{}

		// SSH
		sshEnabled, err := p.GetSSHStatus()
		if err != nil {
			resp["ssh"] = map[string]any{"enabled": false, "error": err.Error()}
		} else {
			resp["ssh"] = map[string]any{"enabled": sshEnabled}
		}

		// Screen sharing (VNC)
		ssEnabled, err := p.GetScreenSharingStatus()
		if errors.Is(err, system.ErrNotSupported) {
			resp["screenSharing"] = map[string]any{"enabled": false, "supported": false}
		} else if err != nil {
			resp["screenSharing"] = map[string]any{"enabled": false, "supported": true, "error": err.Error()}
		} else {
			resp["screenSharing"] = map[string]any{"enabled": ssEnabled, "supported": true}
		}

		// Sessions
		sessions, err := p.GetActiveSessions()
		if err != nil || sessions == nil {
			resp["sessions"] = []system.Session{}
		} else {
			resp["sessions"] = sessions
			// Track new SSH sessions (skip first poll to avoid logging existing sessions on restart)
			sessionMu.Lock()
			currentKeys := map[string]bool{}
			for _, s := range sessions {
				key := s.User + "@" + s.Source
				currentKeys[key] = true
				if !firstSessionPoll && !lastSessionKeys[key] && s.Source != "" {
					LogActivity("access", fmt.Sprintf("SSH session: %s from %s", s.User, s.Source))
				}
			}
			lastSessionKeys = currentKeys
			firstSessionPoll = false
			sessionMu.Unlock()
		}

		// Tailscale devices
		tsStatus, err := tailscale.GetStatus()
		if err != nil {
			resp["devices"] = []any{}
		} else {
			resp["devices"] = tsStatus.Devices
		}

		// Tmux sessions
		tmuxSessions := getTmuxSessions()
		if tmuxSessions == nil {
			resp["tmux"] = []tmuxSession{}
		} else {
			resp["tmux"] = tmuxSessions
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func handleSetSSH(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetSSH(req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("SSH %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetScreenSharing(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetScreenSharing(req.Enabled); err != nil {
			if errors.Is(err, system.ErrNotSupported) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not supported on this platform"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("Screen sharing %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleKillTmuxSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session name"})
		return
	}

	// Find console user
	userOut, err := exec.Command("stat", "-f", "%Su", "/dev/console").Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot determine user"})
		return
	}
	user := strings.TrimSpace(string(userOut))

	err = exec.Command("su", "-", user, "-c", "tmux kill-session -t "+req.Name).Run()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	LogActivity("system", "Killed tmux session: "+req.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleInstallScreenSharing(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		LogActivity("system", "VNC server install requested")
		if err := p.InstallScreenSharing(); err != nil {
			if errors.Is(err, system.ErrNotSupported) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not supported on this platform"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

