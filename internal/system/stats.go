package system

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type SystemInfo struct {
	OS          string  `json:"os"`
	OSVersion   string  `json:"osVersion"`
	Hostname    string  `json:"hostname"`
	Uptime      uint64  `json:"uptimeSeconds"`
	UptimeStr   string  `json:"uptime"`
	CPUPercent  float64 `json:"cpuPercent"`
	CPUModel    string  `json:"cpuModel"`
	CPUCores    int     `json:"cpuCores"`
	MemTotalGB  float64 `json:"memTotalGB"`
	MemUsedGB   float64 `json:"memUsedGB"`
	MemPercent  float64 `json:"memPercent"`
	DiskTotalGB float64 `json:"diskTotalGB"`
	DiskUsedGB  float64 `json:"diskUsedGB"`
	DiskPercent float64 `json:"diskPercent"`
}

// cpuCache holds the most recent CPU percentage, updated by a background goroutine.
var (
	cpuCache   float64
	cpuCacheMu sync.RWMutex
)

// StartCPUMonitor begins a background goroutine that samples CPU usage every 2 seconds.
func StartCPUMonitor() {
	go func() {
		for {
			pct, err := cpu.Percent(2*time.Second, false)
			if err == nil && len(pct) > 0 {
				cpuCacheMu.Lock()
				cpuCache = pct[0]
				cpuCacheMu.Unlock()
			}
		}
	}()
}

func getCPUPercent() float64 {
	cpuCacheMu.RLock()
	defer cpuCacheMu.RUnlock()
	return cpuCache
}

func GetSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{}

	// OS
	info.OS = runtime.GOOS
	if hi, err := host.Info(); err == nil {
		info.OSVersion = hi.PlatformVersion
		info.Hostname = hi.Hostname
		if hi.Platform != "" {
			info.OS = hi.Platform + " " + hi.PlatformVersion
		}
	}

	// Uptime
	if uptime, err := host.Uptime(); err == nil {
		info.Uptime = uptime
		info.UptimeStr = formatUptime(uptime)
	}

	// CPU
	info.CPUPercent = getCPUPercent()
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		info.CPUModel = cpuInfo[0].ModelName
	}
	info.CPUCores = runtime.NumCPU()

	// Memory
	if vm, err := mem.VirtualMemory(); err == nil {
		info.MemTotalGB = float64(vm.Total) / (1 << 30)
		info.MemUsedGB = float64(vm.Used) / (1 << 30)
		info.MemPercent = vm.UsedPercent
	}

	// Disk (root partition)
	root := "/"
	if du, err := disk.Usage(root); err == nil {
		info.DiskTotalGB = float64(du.Total) / (1 << 30)
		info.DiskUsedGB = float64(du.Used) / (1 << 30)
		info.DiskPercent = du.UsedPercent
	}

	return info, nil
}

func formatUptime(seconds uint64) string {
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
