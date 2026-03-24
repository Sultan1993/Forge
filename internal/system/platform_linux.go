//go:build linux

package system

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type linuxPlatform struct{}

func NewPlatform() Platform {
	return &linuxPlatform{}
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// --- Sleep ---

var sleepTargets = []string{
	"sleep.target",
	"suspend.target",
	"hibernate.target",
	"hybrid-sleep.target",
}

func (l *linuxPlatform) GetSleepSettings() (*SleepSettings, error) {
	// Check if sleep targets are masked
	masked := true
	for _, target := range sleepTargets {
		out, _ := runCmd("systemctl", "is-enabled", target)
		if !strings.Contains(out, "masked") {
			masked = false
			break
		}
	}
	return &SleepSettings{
		SleepEnabled:        !masked,
		DisplaySleepEnabled: !masked,
		DiskSleepEnabled:    !masked,
	}, nil
}

func (l *linuxPlatform) SetSleep(enabled bool) error {
	action := "mask"
	if enabled {
		action = "unmask"
	}
	args := append([]string{action}, sleepTargets...)
	_, err := runCmd("sudo", append([]string{"systemctl"}, args...)...)
	return err
}

func (l *linuxPlatform) SetDisplaySleep(enabled bool) error {
	// On Linux, display sleep is controlled by the same targets
	return l.SetSleep(enabled)
}

func (l *linuxPlatform) SetDiskSleep(enabled bool) error {
	// On Linux, disk sleep is controlled by the same targets
	return l.SetSleep(enabled)
}

// --- Power actions ---

func (l *linuxPlatform) Restart() error {
	_, err := runCmd("sudo", "shutdown", "-r", "now")
	return err
}

func (l *linuxPlatform) Shutdown() error {
	_, err := runCmd("sudo", "shutdown", "-h", "now")
	return err
}

// --- Power schedule ---

const (
	sleepTimerPath = "/etc/systemd/system/forge-sleep.timer"
	sleepSvcPath   = "/etc/systemd/system/forge-sleep.service"
)

func (l *linuxPlatform) GetPowerSchedule() (*PowerSchedule, error) {
	schedule := &PowerSchedule{}

	// Check if sleep timer is enabled
	sleepOut, _ := runCmd("systemctl", "is-enabled", "forge-sleep.timer")
	if strings.TrimSpace(sleepOut) == "enabled" {
		schedule.Enabled = true
	}

	// Parse sleep time from timer
	if timerContent, err := exec.Command("cat", sleepTimerPath).Output(); err == nil {
		for _, line := range strings.Split(string(timerContent), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "OnCalendar=") {
				val := strings.TrimPrefix(line, "OnCalendar=")
				parts := strings.Fields(val)
				if len(parts) >= 2 && len(parts[1]) >= 5 {
					schedule.SleepTime = parts[1][:5]
				}
			}
		}
	}

	// Parse wake time from sleep service (embedded in rtcwake command)
	if svcContent, err := exec.Command("cat", sleepSvcPath).Output(); err == nil {
		for _, line := range strings.Split(string(svcContent), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "rtcwake") && strings.Contains(line, "tomorrow") {
				// Extract HH:MM from: ... date -d "tomorrow HH:MM" ...
				if idx := strings.Index(line, "tomorrow "); idx >= 0 {
					rest := line[idx+9:]
					if len(rest) >= 5 {
						schedule.WakeTime = rest[:5]
					}
				}
			}
		}
	}

	return schedule, nil
}

func (l *linuxPlatform) SetPowerSchedule(schedule *PowerSchedule) error {
	if !schedule.Enabled {
		// Disable and remove timer + service
		runCmd("sudo", "systemctl", "stop", "forge-sleep.timer")
		runCmd("sudo", "systemctl", "disable", "forge-sleep.timer")
		exec.Command("sudo", "rm", "-f", sleepTimerPath, sleepSvcPath).Run()
		runCmd("sudo", "systemctl", "daemon-reload")
		return nil
	}

	// Create a sleep service that sets the wake alarm (if configured) then suspends
	sleepExec := "/usr/bin/systemctl suspend"
	if schedule.WakeTime != "" {
		// Set RTC wake alarm before suspending
		sleepExec = fmt.Sprintf("/bin/sh -c '/usr/sbin/rtcwake -m no -t $$(date -d \"tomorrow %s\" +%%%%s) && /usr/bin/systemctl suspend'", schedule.WakeTime)
	}

	if schedule.SleepTime != "" {
		svc := fmt.Sprintf("[Unit]\nDescription=Forge scheduled sleep\n\n[Service]\nType=oneshot\nExecStart=%s\n", sleepExec)
		cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", sleepSvcPath))
		cmd.Stdin = strings.NewReader(svc)
		cmd.Run()

		timer := fmt.Sprintf("[Unit]\nDescription=Forge sleep schedule\n\n[Timer]\nOnCalendar=*-*-* %s:00\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", schedule.SleepTime)
		cmd = exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", sleepTimerPath))
		cmd.Stdin = strings.NewReader(timer)
		cmd.Run()

		runCmd("sudo", "systemctl", "daemon-reload")
		runCmd("sudo", "systemctl", "enable", "--now", "forge-sleep.timer")
	}

	return nil
}

// --- Wake-on-LAN ---

func (l *linuxPlatform) GetWakeOnLANStatus() (*WakeOnLANStatus, error) {
	status := &WakeOnLANStatus{}

	// Find primary network interface
	iface := findPrimaryInterface()
	if iface == "" {
		return status, nil
	}

	// Check WoL status via ethtool
	out, err := runCmd("ethtool", iface)
	if err != nil {
		return status, nil
	}
	status.Supported = true
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Wake-on:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Wake-on:"))
			status.Enabled = strings.Contains(val, "g")
		}
	}

	// Get MAC address
	macOut, err := runCmd("ip", "link", "show", iface)
	if err == nil {
		for _, line := range strings.Split(macOut, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "link/ether") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					status.MACAddr = parts[1]
				}
			}
		}
	}

	return status, nil
}

func (l *linuxPlatform) SetWakeOnLAN(enabled bool) error {
	iface := findPrimaryInterface()
	if iface == "" {
		return fmt.Errorf("no network interface found")
	}
	val := "d"
	if enabled {
		val = "g"
	}
	_, err := runCmd("sudo", "ethtool", "-s", iface, "wol", val)
	return err
}

func findPrimaryInterface() string {
	// Find the default route interface
	out, err := runCmd("ip", "route", "show", "default")
	if err != nil {
		return ""
	}
	// "default via 192.168.1.1 dev eth0 ..."
	parts := strings.Fields(out)
	for i, p := range parts {
		if p == "dev" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// --- Auto-login ---

func (l *linuxPlatform) GetAutoLoginStatus() (*AutoLoginStatus, error) {
	return &AutoLoginStatus{Supported: false}, nil
}

func (l *linuxPlatform) SetAutoLogin(enabled bool) error {
	return ErrNotSupported
}

// --- SSH ---

func (l *linuxPlatform) GetSSHStatus() (bool, error) {
	// Try sshd first (most distros), then ssh (Ubuntu/Debian)
	for _, svc := range []string{"sshd", "ssh"} {
		out, err := runCmd("systemctl", "is-active", svc)
		if err == nil && strings.TrimSpace(out) == "active" {
			return true, nil
		}
	}
	return false, nil
}

func (l *linuxPlatform) SetSSH(enabled bool) error {
	action := "stop"
	if enabled {
		action = "start"
	}
	// Try sshd first, fall back to ssh
	for _, svc := range []string{"sshd", "ssh"} {
		_, err := runCmd("sudo", "systemctl", action, svc)
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not %s SSH service", action)
}

// --- Screen sharing (VNC) ---

// Known VNC services: unit is what we start/stop, search is what we look for in list-unit-files.
// Template units (containing @) need the template name (without instance) for lookup.
var vncServices = []struct {
	unit   string // used for start/stop/is-active (instance name)
	search string // used for list-unit-files (template name)
}{
	{"vncserver@:1", "vncserver@"},   // TigerVNC (RHEL/Fedora)
	{"tigervnc@:1", "tigervnc@"},    // TigerVNC (alternate)
	{"x11vnc", "x11vnc"},            // x11vnc
	{"vino-server", "vino-server"},   // GNOME Vino
}

// findVNCService returns the first installed VNC systemd unit name, or "" if none found.
func findVNCService() string {
	for _, svc := range vncServices {
		out, _ := runCmd("systemctl", "list-unit-files", svc.search+".service")
		if strings.Contains(out, svc.search) {
			return svc.unit
		}
	}
	return ""
}

func (l *linuxPlatform) GetScreenSharingStatus() (bool, error) {
	svc := findVNCService()
	if svc == "" {
		return false, ErrNotSupported
	}
	out, err := runCmd("systemctl", "is-active", svc)
	if err == nil && strings.TrimSpace(out) == "active" {
		return true, nil
	}
	return false, nil
}

func (l *linuxPlatform) SetScreenSharing(enabled bool) error {
	svc := findVNCService()
	if svc == "" {
		return ErrNotSupported
	}
	action := "stop"
	enableAction := "disable"
	if enabled {
		action = "start"
		enableAction = "enable"
	}
	// Enable/disable for persistence, then start/stop for immediate effect
	runCmd("sudo", "systemctl", enableAction, svc)
	_, err := runCmd("sudo", "systemctl", action, svc)
	return err
}

func (l *linuxPlatform) InstallScreenSharing() error {
	// Try apt (Debian/Ubuntu)
	if _, err := runCmd("which", "apt-get"); err == nil {
		_, err := runCmd("sudo", "apt-get", "update", "-qq")
		if err != nil {
			return fmt.Errorf("apt-get update failed: %w", err)
		}
		// Try tigervnc first, fall back to x11vnc
		if _, err := runCmd("sudo", "apt-get", "install", "-y", "-qq", "tigervnc-standalone-server"); err == nil {
			ensureVNCServiceFile("tigervnc")
			return nil
		}
		if _, err := runCmd("sudo", "apt-get", "install", "-y", "-qq", "x11vnc"); err == nil {
			ensureVNCServiceFile("x11vnc")
			return nil
		}
		return fmt.Errorf("failed to install VNC server via apt")
	}

	// Try dnf (Fedora/RHEL) — includes systemd unit
	if _, err := runCmd("which", "dnf"); err == nil {
		if _, err := runCmd("sudo", "dnf", "install", "-y", "-q", "tigervnc-server"); err == nil {
			return nil
		}
		return fmt.Errorf("failed to install VNC server via dnf")
	}

	// Try pacman (Arch)
	if _, err := runCmd("which", "pacman"); err == nil {
		if _, err := runCmd("sudo", "pacman", "-S", "--noconfirm", "tigervnc"); err == nil {
			return nil
		}
		return fmt.Errorf("failed to install VNC server via pacman")
	}

	return fmt.Errorf("no supported package manager found (tried apt, dnf, pacman)")
}

// ensureVNCServiceFile creates a systemd service file if the package didn't include one.
// Debian/Ubuntu tigervnc-standalone-server and x11vnc don't ship systemd units.
func ensureVNCServiceFile(variant string) {
	// Check if a service already exists
	if findVNCService() != "" {
		return
	}

	var unit, content string
	switch variant {
	case "tigervnc":
		unit = "/etc/systemd/system/tigervnc@.service"
		content = `[Unit]
Description=TigerVNC server on display %i
After=syslog.target network.target

[Service]
Type=simple
User=root
ExecStart=/usr/bin/Xtigervnc :%i -geometry 1280x720 -SecurityTypes None -localhost no -AlwaysShared
Restart=on-failure

[Install]
WantedBy=multi-user.target
`
	case "x11vnc":
		unit = "/etc/systemd/system/x11vnc.service"
		content = `[Unit]
Description=x11vnc VNC server
After=display-manager.service network.target

[Service]
Type=simple
ExecStart=/usr/bin/x11vnc -display :0 -forever -shared -nopw
Restart=on-failure

[Install]
WantedBy=multi-user.target
`
	default:
		return
	}

	// Write unit file and reload systemd
	cmd := exec.Command("sudo", "sh", "-c", fmt.Sprintf("cat > %s", unit))
	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err != nil {
		return
	}
	runCmd("sudo", "systemctl", "daemon-reload")
}

// --- Sessions ---

func (l *linuxPlatform) GetActiveSessions() ([]Session, error) {
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

		// Extract source IP from parentheses
		for _, f := range fields {
			if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
				s.Source = strings.Trim(f, "()")
			}
		}

		// Try to parse login time
		if len(fields) >= 4 {
			timeStr := fields[2] + " " + fields[3]
			if t, err := time.Parse("2006-01-02 15:04", timeStr); err == nil {
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
