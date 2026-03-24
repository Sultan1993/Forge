package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Sultan1993/forge/internal/system"
)

type toggleRequest struct {
	Enabled bool `json:"enabled"`
}

func handleGetPower(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sleep, err := p.GetSleepSettings()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		autoLogin, err := p.GetAutoLoginStatus()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		wol, _ := p.GetWakeOnLANStatus()
		sched, _ := p.GetPowerSchedule()
		writeJSON(w, http.StatusOK, map[string]any{
			"sleep":     sleep,
			"autoLogin": autoLogin,
			"wol":       wol,
			"schedule":  sched,
		})
	}
}

func handleSetSleep(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetSleep(req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("System sleep %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetDisplaySleep(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetDisplaySleep(req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("Display sleep %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetDiskSleep(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetDiskSleep(req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("Disk sleep %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetAutoLogin(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetAutoLogin(req.Enabled); err != nil {
			if errors.Is(err, system.ErrNotSupported) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not supported on this platform"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("Auto-login %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetSchedule(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req system.PowerSchedule
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetPowerSchedule(&req); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if req.Enabled {
			LogActivity("toggle", fmt.Sprintf("Power schedule set: sleep %s, wake %s", req.SleepTime, req.WakeTime))
		} else {
			LogActivity("toggle", "Power schedule disabled")
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetWakeOnLAN(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toggleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := p.SetWakeOnLAN(req.Enabled); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		LogActivity("toggle", fmt.Sprintf("Wake-on-LAN %s", enabledStr(req.Enabled)))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleRestart(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		LogActivity("power", "Machine restart requested")
		writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
		go p.Restart()
	}
}

func handleShutdown(p system.Platform) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		LogActivity("power", "Machine shutdown requested")
		writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
		go p.Shutdown()
	}
}

func enabledStr(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
