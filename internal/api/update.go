package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

type updateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	UpdateAvail    bool   `json:"updateAvailable"`
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	latest, err := fetchLatestVersion()
	if err != nil {
		writeJSON(w, http.StatusOK, updateInfo{
			CurrentVersion: appVersion,
			LatestVersion:  "",
			UpdateAvail:    false,
		})
		return
	}

	writeJSON(w, http.StatusOK, updateInfo{
		CurrentVersion: appVersion,
		LatestVersion:  latest,
		UpdateAvail:    latest != appVersion && latest != "v"+appVersion,
	})
}

func handleDoUpdate(repo string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		latest, err := fetchLatestVersion()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if latest == appVersion || latest == "v"+appVersion {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already up to date"})
			return
		}

		// Respond before updating (the daemon will restart)
		LogActivity("system", "Update to "+latest+" started")
		writeJSON(w, http.StatusOK, map[string]string{"status": "updating", "version": latest})

		go func() {
			time.Sleep(500 * time.Millisecond)
			downloadAndReplace(repo, latest)
		}()
	}
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + updateRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// updateRepo is set by main via SetUpdateRepo.
var updateRepo string

func SetUpdateRepo(repo string) {
	updateRepo = repo
}

func downloadAndReplace(repo, version string) {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Download new daemon
	daemonURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/forge-host-%s-%s", repo, version, os, arch)
	trayURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/forge-host-tray-%s-%s", repo, version, os, arch)

	// Download daemon
	exec.Command("sudo", "curl", "-fsSL", "-o", "/usr/local/bin/forge-host.new", daemonURL).Run()
	exec.Command("sudo", "chmod", "+x", "/usr/local/bin/forge-host.new").Run()
	exec.Command("sudo", "mv", "/usr/local/bin/forge-host.new", "/usr/local/bin/forge-host").Run()

	// Download tray
	exec.Command("sudo", "curl", "-fsSL", "-o", "/usr/local/bin/forge-host-tray.new", trayURL).Run()
	exec.Command("sudo", "chmod", "+x", "/usr/local/bin/forge-host-tray.new").Run()
	exec.Command("sudo", "mv", "/usr/local/bin/forge-host-tray.new", "/usr/local/bin/forge-host-tray").Run()

	// Restart daemon
	if runtime.GOOS == "darwin" {
		exec.Command("sudo", "launchctl", "kickstart", "-k", "system/dev.forge").Run()
	} else {
		exec.Command("sudo", "systemctl", "restart", "forge").Run()
	}

	// Restart tray
	exec.Command("pkill", "forge-host-tray").Run()
}
