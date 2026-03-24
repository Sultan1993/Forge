package api

import (
	"net/http"
	"time"

	"github.com/Sultan1993/forge/internal/system"
)

func handleUninstall() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Respond first, then uninstall (which stops the daemon)
		LogActivity("system", "Uninstall requested")
		writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalling"})

		go func() {
			time.Sleep(500 * time.Millisecond)
			system.Uninstall()
		}()
	}
}
