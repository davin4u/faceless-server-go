package routes

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

var startTime = time.Now()

type systemInfo struct {
	RAMUsedMB     uint64     `json:"ramUsedMb"`
	RAMTotalMB    uint64     `json:"ramTotalMb"`
	CPULoadAvg    [3]float64 `json:"cpuLoadAvg"`
	DiskUsedGB    float64    `json:"diskUsedGb"`
	DiskTotalGB   float64    `json:"diskTotalGb"`
	UptimeSeconds int64      `json:"uptimeSeconds"`
	NodeVersion   string     `json:"nodeVersion"` // kept name "nodeVersion" for admin dashboard parity; value is Go runtime
}

func collectSystem() systemInfo {
	out := systemInfo{
		UptimeSeconds: int64(time.Since(startTime).Seconds()),
		NodeVersion:   runtime.Version(),
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		out.RAMUsedMB = (vm.Total - vm.Available) / (1024 * 1024)
		out.RAMTotalMB = vm.Total / (1024 * 1024)
	}
	if l, err := load.AvgWithContext(context.Background()); err == nil {
		out.CPULoadAvg = [3]float64{l.Load1, l.Load5, l.Load15}
	}
	if u, err := disk.UsageWithContext(context.Background(), "/"); err == nil {
		out.DiskTotalGB = roundOneDecimal(float64(u.Total) / (1024 * 1024 * 1024))
		out.DiskUsedGB = roundOneDecimal(float64(u.Used) / (1024 * 1024 * 1024))
	}
	return out
}

func roundOneDecimal(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10.0
}
