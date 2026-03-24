//go:build linux

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Uninstall() error {
	home, _ := os.UserHomeDir()
	var errs []string

	// Stop and remove tray app
	exec.Command("pkill", "forge-host-tray").Run()
	os.Remove(filepath.Join(home, ".config", "autostart", "forge-host-tray.desktop"))

	// Stop and remove daemon service
	exec.Command("sudo", "systemctl", "stop", "forge").Run()
	exec.Command("sudo", "systemctl", "disable", "forge").Run()
	os.Remove("/etc/systemd/system/forge.service")
	exec.Command("sudo", "systemctl", "daemon-reload").Run()

	// Remove binaries
	if err := os.Remove("/usr/local/bin/forge-host"); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove forge-host: %v", err))
	}
	if err := os.Remove("/usr/local/bin/forge-host-tray"); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove forge-host-tray: %v", err))
	}

	// Remove config
	os.RemoveAll(filepath.Join(home, ".forge"))

	if len(errs) > 0 {
		return fmt.Errorf("uninstall completed with errors: %v", errs)
	}
	return nil
}
