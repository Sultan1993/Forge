//go:build darwin

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
	trayPlist := filepath.Join(home, "Library", "LaunchAgents", "dev.forge.tray.plist")
	exec.Command("launchctl", "unload", "-w", trayPlist).Run()
	os.Remove(trayPlist)
	exec.Command("pkill", "forge-host-tray").Run()

	// Stop and remove daemon service
	exec.Command("sudo", "launchctl", "bootout", "system/dev.forge").Run()
	os.Remove("/Library/LaunchDaemons/dev.forge.plist")

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
