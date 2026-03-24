package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ActivityEvent struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

var (
	activityMu   sync.Mutex
	activityFile string
)

func initActivity() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".forge")
	os.MkdirAll(dir, 0755)
	activityFile = filepath.Join(dir, "activity.log")

	// Rotate if older than 7 days
	info, err := os.Stat(activityFile)
	if err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
		os.Rename(activityFile, activityFile+".old")
	}
}

// LogActivity appends an event to the activity log.
func LogActivity(action, detail string) {
	if activityFile == "" {
		return
	}
	event := ActivityEvent{
		Time:   time.Now().Format(time.RFC3339),
		Action: action,
		Detail: detail,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')

	activityMu.Lock()
	defer activityMu.Unlock()

	f, err := os.OpenFile(activityFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

func handleGetActivity(w http.ResponseWriter, r *http.Request) {
	if activityFile == "" {
		writeJSON(w, http.StatusOK, []ActivityEvent{})
		return
	}

	data, err := os.ReadFile(activityFile)
	if err != nil {
		writeJSON(w, http.StatusOK, []ActivityEvent{})
		return
	}

	// Parse all lines, return last 50
	var events []ActivityEvent
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e ActivityEvent
		if json.Unmarshal(line, &e) == nil {
			events = append(events, e)
		}
	}

	// Take last 50
	if len(events) > 50 {
		events = events[len(events)-50:]
	}

	// Reverse — newest first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	writeJSON(w, http.StatusOK, events)
}

// TrackDashboardVisit logs a dashboard visit, deduped per IP per hour.
var (
	visitCache   = map[string]time.Time{}
	visitCacheMu sync.Mutex
)

func TrackDashboardVisit(r *http.Request) {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i >= 0 {
		ip = ip[:i]
	}

	visitCacheMu.Lock()
	defer visitCacheMu.Unlock()

	if last, ok := visitCache[ip]; ok && time.Since(last) < time.Hour {
		return
	}
	visitCache[ip] = time.Now()
	LogActivity("access", "Dashboard opened from "+ip)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
