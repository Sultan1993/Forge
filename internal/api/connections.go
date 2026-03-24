package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/Sultan1993/forge/internal/system"
	"github.com/Sultan1993/forge/internal/tailscale"
)

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

