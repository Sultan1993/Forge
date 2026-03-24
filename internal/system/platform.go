package system

import "errors"

var ErrNotSupported = errors.New("not supported on this platform")

type SleepSettings struct {
	SleepEnabled        bool `json:"sleepEnabled"`
	DisplaySleepEnabled bool `json:"displaySleepEnabled"`
	DiskSleepEnabled    bool `json:"diskSleepEnabled"`
}

type PowerSchedule struct {
	Enabled   bool   `json:"enabled"`
	SleepTime string `json:"sleepTime,omitempty"` // "HH:MM" or ""
	WakeTime  string `json:"wakeTime,omitempty"`  // "HH:MM" or ""
}

type WakeOnLANStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	MACAddr   string `json:"macAddr,omitempty"`
}

type AutoLoginStatus struct {
	Supported       bool   `json:"supported"`
	Enabled         bool   `json:"enabled"`
	FileVaultActive bool   `json:"fileVaultActive"`
	User            string `json:"user,omitempty"`
}

type Session struct {
	User     string `json:"user"`
	Source   string `json:"source"`
	Duration string `json:"duration"`
}

type Platform interface {
	// Power / sleep
	GetSleepSettings() (*SleepSettings, error)
	SetSleep(enabled bool) error
	SetDisplaySleep(enabled bool) error
	SetDiskSleep(enabled bool) error
	Restart() error
	Shutdown() error

	// Power schedule
	GetPowerSchedule() (*PowerSchedule, error)
	SetPowerSchedule(schedule *PowerSchedule) error

	// Wake-on-LAN
	GetWakeOnLANStatus() (*WakeOnLANStatus, error)
	SetWakeOnLAN(enabled bool) error

	// Auto-login
	GetAutoLoginStatus() (*AutoLoginStatus, error)
	SetAutoLogin(enabled bool) error

	// Connections
	GetSSHStatus() (bool, error)
	SetSSH(enabled bool) error
	GetScreenSharingStatus() (bool, error)
	SetScreenSharing(enabled bool) error
	InstallScreenSharing() error
	GetActiveSessions() ([]Session, error)
}
