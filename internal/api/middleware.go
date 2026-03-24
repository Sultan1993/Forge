package api

import "net/http"

// RequireConfirm wraps a handler and rejects POST requests missing the
// X-Forge-Confirm: true header. Acts as a CSRF guard.
func RequireConfirm(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forge-Confirm") != "true" {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "missing X-Forge-Confirm header",
			})
			return
		}
		next(w, r)
	}
}

// JSONContentType is a pass-through middleware placeholder.
// Content-Type is set by writeJSON for API responses to avoid double-setting.
func JSONContentType(next http.Handler) http.Handler {
	return next
}
