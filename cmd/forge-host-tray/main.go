package main

import (
	_ "embed"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/systray"
)

//go:embed icon.png
var iconBytes []byte

func main() {
	systray.Run(onReady, func() {})
}

func onReady() {
	systray.SetTemplateIcon(iconBytes, iconBytes)
	systray.SetTooltip("Forge")

	mOpen := systray.AddMenuItem("Open Dashboard", "Open the Forge web dashboard")
	systray.AddSeparator()
	mAutoStart := systray.AddMenuItemCheckbox("Auto-start on login", "Start Forge when you log in", isAutoStartEnabled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Forge", "Stop the Forge daemon and quit")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openDashboard()
			case <-mAutoStart.ClickedCh:
				if mAutoStart.Checked() {
					disableAutoStart()
					mAutoStart.Uncheck()
				} else {
					enableAutoStart()
					mAutoStart.Check()
				}
			case <-mQuit.ClickedCh:
				stopDaemon()
				systray.Quit()
			}
		}
	}()
}

// --- Open Dashboard ---

func openDashboard() {
	ip := tailscaleIP()
	if ip == "" {
		ip = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s:8080", ip)

	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", url)
	} else {
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func tailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				if strings.HasPrefix(ipnet.IP.String(), "100.") {
					return ipnet.IP.String()
				}
			}
		}
	}
	return ""
}

// --- Auto-start ---

func isAutoStartEnabled() bool {
	if runtime.GOOS == "darwin" {
		// Check if the launchd plist has KeepAlive/RunAtLoad
		out, err := exec.Command("launchctl", "print", "system/dev.forge").CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "state = running")
	}
	// Linux: check if systemd service is enabled
	out, _ := exec.Command("systemctl", "is-enabled", "forge").CombinedOutput()
	return strings.TrimSpace(string(out)) == "enabled"
}

func enableAutoStart() {
	if runtime.GOOS == "darwin" {
		// Enable daemon
		exec.Command("sudo", "launchctl", "enable", "system/dev.forge").Run()
		// Enable tray login item
		enableLoginItem()
	} else {
		exec.Command("sudo", "systemctl", "enable", "forge").Run()
		enableLinuxAutostart()
	}
}

func disableAutoStart() {
	if runtime.GOOS == "darwin" {
		// Disable daemon
		exec.Command("sudo", "launchctl", "disable", "system/dev.forge").Run()
		// Disable tray login item
		disableLoginItem()
	} else {
		exec.Command("sudo", "systemctl", "disable", "forge").Run()
		disableLinuxAutostart()
	}
}

// macOS login item for the tray app
func enableLoginItem() {
	home, _ := exec.Command("sh", "-c", "echo $HOME").Output()
	plist := strings.TrimSpace(string(home)) + "/Library/LaunchAgents/dev.forge.tray.plist"
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>dev.forge.tray</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/forge-host-tray</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`)
	exec.Command("sh", "-c", fmt.Sprintf("mkdir -p ~/Library/LaunchAgents && cat > %s", plist)).
		Run()
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s", plist))
	cmd.Stdin = strings.NewReader(content)
	cmd.Run()
	exec.Command("launchctl", "load", "-w", plist).Run()
}

func disableLoginItem() {
	home, _ := exec.Command("sh", "-c", "echo $HOME").Output()
	plist := strings.TrimSpace(string(home)) + "/Library/LaunchAgents/dev.forge.tray.plist"
	exec.Command("launchctl", "unload", "-w", plist).Run()
}

// Linux autostart for the tray app
func enableLinuxAutostart() {
	content := `[Desktop Entry]
Type=Application
Name=Forge Tray
Exec=/usr/local/bin/forge-host-tray
X-GNOME-Autostart-enabled=true
`
	dir := exec.Command("sh", "-c", "echo $HOME/.config/autostart")
	dirOut, _ := dir.Output()
	dirPath := strings.TrimSpace(string(dirOut))
	exec.Command("mkdir", "-p", dirPath).Run()
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s/forge-host-tray.desktop", dirPath))
	cmd.Stdin = strings.NewReader(content)
	cmd.Run()
}

func disableLinuxAutostart() {
	home, _ := exec.Command("sh", "-c", "echo $HOME").Output()
	path := strings.TrimSpace(string(home)) + "/.config/autostart/forge-host-tray.desktop"
	exec.Command("rm", "-f", path).Run()
}

// --- Quit ---

func stopDaemon() {
	if runtime.GOOS == "darwin" {
		// Use osascript to get admin privileges for stopping the system daemon
		exec.Command("osascript", "-e",
			`do shell script "launchctl bootout system/dev.forge" with administrator privileges`).Run()
	} else {
		exec.Command("sudo", "systemctl", "stop", "forge").Run()
	}
}
