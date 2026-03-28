package api

import (
	"encoding/json"
	"net/http"

	"github.com/Sultan1993/forge/internal/system"
	"github.com/Sultan1993/forge/internal/web"
)

// NewRouter creates the HTTP mux with all API routes registered.
func NewRouter(p system.Platform) http.Handler {
	initActivity()
	LogActivity("daemon", "Forge started")

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /api/health", handleHealth)

	// System
	mux.HandleFunc("GET /api/system", handleSystem)
	mux.HandleFunc("GET /api/version", handleVersion)
	mux.HandleFunc("GET /api/processes", handleProcesses)
	mux.HandleFunc("POST /api/processes/kill", RequireConfirm(handleKillProcess))
	mux.HandleFunc("GET /api/environment", handleEnvironment)

	// Power
	mux.HandleFunc("GET /api/power", handleGetPower(p))
	mux.HandleFunc("POST /api/power/sleep", RequireConfirm(handleSetSleep(p)))
	mux.HandleFunc("POST /api/power/display-sleep", RequireConfirm(handleSetDisplaySleep(p)))
	mux.HandleFunc("POST /api/power/disk-sleep", RequireConfirm(handleSetDiskSleep(p)))
	mux.HandleFunc("POST /api/power/auto-login", RequireConfirm(handleSetAutoLogin(p)))
	mux.HandleFunc("POST /api/power/schedule", RequireConfirm(handleSetSchedule(p)))
	mux.HandleFunc("POST /api/power/wol", RequireConfirm(handleSetWakeOnLAN(p)))
	mux.HandleFunc("POST /api/power/restart", RequireConfirm(handleRestart(p)))
	mux.HandleFunc("POST /api/power/shutdown", RequireConfirm(handleShutdown(p)))

	// Connections
	mux.HandleFunc("GET /api/connections", handleGetConnections(p))
	mux.HandleFunc("POST /api/connections/ssh", RequireConfirm(handleSetSSH(p)))
	mux.HandleFunc("POST /api/connections/screensharing", RequireConfirm(handleSetScreenSharing(p)))
	mux.HandleFunc("POST /api/connections/screensharing/install", RequireConfirm(handleInstallScreenSharing(p)))

	// Files
	mux.HandleFunc("GET /api/files/list", handleFilesList)
	mux.HandleFunc("POST /api/files/mkdir", RequireConfirm(handleFilesMkdir))
	mux.HandleFunc("POST /api/files/rename", RequireConfirm(handleFilesRename))
	mux.HandleFunc("POST /api/files/delete", RequireConfirm(handleFilesDelete))
	mux.HandleFunc("GET /api/files/download", handleFilesDownload)
	mux.HandleFunc("POST /api/files/upload", RequireConfirm(handleFilesUpload))

	// Activity
	mux.HandleFunc("GET /api/activity", handleGetActivity)

	// Update
	mux.HandleFunc("GET /api/update/check", handleCheckUpdate)
	mux.HandleFunc("POST /api/update", RequireConfirm(handleDoUpdate(updateRepo)))

	// Uninstall
	mux.HandleFunc("POST /api/uninstall", RequireConfirm(handleUninstall()))

	// Web UI (serve index.html at root, track visits)
	webHandler := web.Handler()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		TrackDashboardVisit(r)
		webHandler.ServeHTTP(w, r)
	})
	mux.Handle("/", webHandler)

	return JSONContentType(mux)
}

// writeJSON is a helper to write a JSON response with a status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
