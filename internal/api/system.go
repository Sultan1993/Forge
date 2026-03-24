package api

import (
	"net/http"

	"github.com/Sultan1993/forge/internal/system"
)

var appVersion = "dev"

// SetVersion sets the version string returned by /api/version.
func SetVersion(v string) {
	appVersion = v
}

func handleSystem(w http.ResponseWriter, r *http.Request) {
	info, err := system.GetSystemInfo()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": appVersion})
}
