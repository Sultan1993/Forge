//go:build darwin

package system

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type darwinPlatform struct{}

func NewPlatform() Platform {
	return &darwinPlatform{}
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// --- Sleep ---

func (d *darwinPlatform) GetSleepSettings() (*SleepSettings, error) {
	out, err := runCmd("pmset", "-g", "custom")
	if err != nil {
		return nil, fmt.Errorf("pmset: %w", err)
	}
	s := &SleepSettings{
		SleepEnabled:        parsePmsetValue(out, "sleep") > 0,
		DisplaySleepEnabled: parsePmsetValue(out, "displaysleep") > 0,
		DiskSleepEnabled:    parsePmsetValue(out, "disksleep") > 0,
	}
	return s, nil
}

func parsePmsetValue(output, key string) int {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var val int
				fmt.Sscanf(parts[1], "%d", &val)
				return val
			}
		}
	}
	return -1
}

func (d *darwinPlatform) SetSleep(enabled bool) error {
	val := "0"
	if enabled {
		val = "1"
	}
	_, err := runCmd("sudo", "pmset", "-a", "sleep", val)
	return err
}

func (d *darwinPlatform) SetDisplaySleep(enabled bool) error {
	val := "0"
	if enabled {
		val = "10"
	}
	_, err := runCmd("sudo", "pmset", "-a", "displaysleep", val)
	return err
}

func (d *darwinPlatform) SetDiskSleep(enabled bool) error {
	val := "0"
	if enabled {
		val = "10"
	}
	_, err := runCmd("sudo", "pmset", "-a", "disksleep", val)
	return err
}

// --- Power actions ---

func (d *darwinPlatform) Restart() error {
	_, err := runCmd("sudo", "shutdown", "-r", "now")
	return err
}

func (d *darwinPlatform) Shutdown() error {
	_, err := runCmd("sudo", "shutdown", "-h", "now")
	return err
}

// --- Power schedule ---

func (d *darwinPlatform) GetPowerSchedule() (*PowerSchedule, error) {
	schedule := &PowerSchedule{}
	out, err := runCmd("pmset", "-g", "sched")
	if err != nil {
		return schedule, nil
	}

	// Only parse the "Repeating power events:" section, ignore system alarms
	inRepeating := false
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Repeating power events:") {
			inRepeating = true
			continue
		}
		// A new section header ends the repeating section
		if inRepeating && !strings.HasPrefix(line, " ") && trimmed != "" {
			break
		}
		if !inRepeating {
			continue
		}
		if strings.Contains(trimmed, "sleep") {
			if t := extractTime(trimmed); t != "" {
				schedule.SleepTime = t
				schedule.Enabled = true
			}
		}
		if strings.Contains(trimmed, "wake") || strings.Contains(trimmed, "poweron") {
			if t := extractTime(trimmed); t != "" {
				schedule.WakeTime = t
				schedule.Enabled = true
			}
		}
	}

	return schedule, nil
}

func extractTime(line string) string {
	// Find HH:MM:SS pattern
	parts := strings.Fields(line)
	for _, p := range parts {
		if len(p) == 8 && p[2] == ':' && p[5] == ':' {
			return p[:5] // return HH:MM
		}
	}
	return ""
}

func (d *darwinPlatform) SetPowerSchedule(schedule *PowerSchedule) error {
	if !schedule.Enabled {
		_, err := runCmd("sudo", "pmset", "repeat", "cancel")
		return err
	}

	// Build pmset repeat command
	// sudo pmset repeat wakeorpoweron MTWRFSU HH:MM:SS sleep MTWRFSU HH:MM:SS
	days := "MTWRFSU"
	args := []string{"repeat"}

	if schedule.WakeTime != "" {
		args = append(args, "wakeorpoweron", days, schedule.WakeTime+":00")
	}
	if schedule.SleepTime != "" {
		args = append(args, "sleep", days, schedule.SleepTime+":00")
	}

	if len(args) == 1 {
		// No times set, cancel
		_, err := runCmd("sudo", "pmset", "repeat", "cancel")
		return err
	}

	_, err := runCmd("sudo", append([]string{"pmset"}, args...)...)
	return err
}

// --- Wake-on-LAN ---

func (d *darwinPlatform) GetWakeOnLANStatus() (*WakeOnLANStatus, error) {
	status := &WakeOnLANStatus{Supported: true}

	// Check womp (Wake On Magic Packet)
	out, err := runCmd("pmset", "-g", "custom")
	if err == nil {
		status.Enabled = parsePmsetValue(out, "womp") > 0
	}

	// Get MAC address of primary interface
	ifOut, err := runCmd("ifconfig", "en0")
	if err == nil {
		for _, line := range strings.Split(ifOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ether ") {
				status.MACAddr = strings.TrimPrefix(line, "ether ")
				break
			}
		}
	}

	return status, nil
}

func (d *darwinPlatform) SetWakeOnLAN(enabled bool) error {
	val := "0"
	if enabled {
		val = "1"
	}
	_, err := runCmd("sudo", "pmset", "-a", "womp", val)
	return err
}

// --- Auto-login ---

func (d *darwinPlatform) GetAutoLoginStatus() (*AutoLoginStatus, error) {
	status := &AutoLoginStatus{Supported: true}

	// Check FileVault
	fvOut, err := runCmd("fdesetup", "status")
	if err == nil && strings.Contains(fvOut, "FileVault is On") {
		status.FileVaultActive = true
		return status, nil
	}

	// Check current auto-login user
	out, err := runCmd("defaults", "read", "/Library/Preferences/com.apple.loginwindow", "autoLoginUser")
	if err == nil && out != "" {
		status.Enabled = true
		status.User = out
	}

	return status, nil
}

func (d *darwinPlatform) SetAutoLogin(enabled bool) error {
	// Check FileVault first
	fvOut, _ := runCmd("fdesetup", "status")
	if strings.Contains(fvOut, "FileVault is On") {
		return fmt.Errorf("cannot enable auto-login: FileVault is active")
	}

	if enabled {
		// Get the console user (not "root" — the daemon runs as root under launchd)
		user, err := runCmd("stat", "-f", "%Su", "/dev/console")
		if err != nil || user == "" || user == "root" {
			// Fallback: read last logged-in user
			user, err = runCmd("defaults", "read", "/Library/Preferences/com.apple.loginwindow", "lastUserName")
			if err != nil || user == "" {
				return fmt.Errorf("could not determine console user")
			}
		}
		_, err = runCmd("sudo", "defaults", "write", "/Library/Preferences/com.apple.loginwindow", "autoLoginUser", user)
		return err
	}
	_, err := runCmd("sudo", "defaults", "delete", "/Library/Preferences/com.apple.loginwindow", "autoLoginUser")
	return err
}

// --- SSH ---

func (d *darwinPlatform) GetSSHStatus() (bool, error) {
	out, err := runCmd("systemsetup", "-getremotelogin")
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(out), "on"), nil
}

func (d *darwinPlatform) SetSSH(enabled bool) error {
	val := "off"
	if enabled {
		val = "on"
	}
	_, err := runCmd("sudo", "systemsetup", "-setremotelogin", val)
	return err
}

// --- Screen sharing (VNC) ---

const screenSharingPlist = "/System/Library/LaunchDaemons/com.apple.screensharing.plist"

func (d *darwinPlatform) GetScreenSharingStatus() (bool, error) {
	// Exit code 0 = service is loaded (enabled); non-zero = not loaded
	_, err := runCmd("launchctl", "list", "com.apple.screensharing")
	return err == nil, nil
}

func (d *darwinPlatform) SetScreenSharing(enabled bool) error {
	if enabled {
		_, err := runCmd("sudo", "launchctl", "load", "-w", screenSharingPlist)
		return err
	}
	_, err := runCmd("sudo", "launchctl", "unload", "-w", screenSharingPlist)
	return err
}

func (d *darwinPlatform) InstallScreenSharing() error {
	return ErrNotSupported // built into macOS, no install needed
}

// --- Sessions ---

func (d *darwinPlatform) GetActiveSessions() ([]Session, error) {
	out, err := runCmd("who")
	if err != nil {
		return nil, err
	}
	return parseWhoOutput(out), nil
}

func parseWhoOutput(output string) []Session {
	var sessions []Session
	now := time.Now()

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		s := Session{
			User: fields[0],
		}

		// Extract source IP from parentheses if present
		for _, f := range fields {
			if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
				s.Source = strings.Trim(f, "()")
			}
		}

		// Try to parse login time for duration
		// Format varies: "Mar 23 14:30" or similar
		if len(fields) >= 5 {
			timeStr := fields[2] + " " + fields[3] + " " + fields[4]
			if t, err := time.Parse("Jan 2 15:04", timeStr); err == nil {
				t = t.AddDate(now.Year(), 0, 0)
				dur := now.Sub(t)
				if dur > 0 {
					s.Duration = formatDuration(dur)
				}
			}
		}

		sessions = append(sessions, s)
	}
	return sessions
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
