package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"syscall"

	"github.com/shirou/gopsutil/v3/process"
)

type processInfo struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu"`
	MemMB  float32 `json:"memMB"`
	MemPct float32 `json:"memPct"`
	User   string  `json:"user"`
}

func handleProcesses(w http.ResponseWriter, r *http.Request) {
	procs, err := process.Processes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var list []processInfo
	for _, p := range procs {
		name, _ := p.Name()
		if name == "" {
			continue
		}
		cpu, _ := p.CPUPercent()
		memInfo, _ := p.MemoryInfo()
		memPct, _ := p.MemoryPercent()
		user, _ := p.Username()

		var memMB float32
		if memInfo != nil {
			memMB = float32(memInfo.RSS) / (1024 * 1024)
		}

		list = append(list, processInfo{
			PID:    p.Pid,
			Name:   name,
			CPU:    cpu,
			MemMB:  memMB,
			MemPct: memPct,
			User:   user,
		})
	}

	// Sort by query param: ?sort=mem or default cpu
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "mem" {
		sort.Slice(list, func(i, j int) bool {
			return list[i].MemMB > list[j].MemMB
		})
	} else {
		sort.Slice(list, func(i, j int) bool {
			return list[i].CPU > list[j].CPU
		})
	}

	if len(list) > 10 {
		list = list[:10]
	}

	writeJSON(w, http.StatusOK, list)
}

func handleKillProcess(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("pid")
	if pidStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing pid parameter"})
		return
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pid"})
		return
	}

	// Don't allow killing PID 1 or the forge daemon itself
	if pid <= 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot kill system process"})
		return
	}

	if err := syscall.Kill(int(pid), syscall.SIGTERM); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("kill %d: %v", pid, err)})
		return
	}

	LogActivity("system", fmt.Sprintf("Killed process %d", pid))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
