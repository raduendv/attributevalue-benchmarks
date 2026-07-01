package cpu

import (
	"github.com/shirou/gopsutil/cpu"
)

func Total(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice
}

func Idle(t cpu.TimesStat) float64 {
	// Iowait is treated as idle for usage calculations to match common CPU-utilization formulas.
	return t.Idle + t.Iowait
}

func UsageBetween(start, end cpu.TimesStat) float64 {
	totalDelta := Total(end) - Total(start)
	idleDelta := Idle(end) - Idle(start)
	if totalDelta <= 0 {
		return 0
	}
	return (totalDelta - idleDelta) / totalDelta * 100
}

func TotalBetween(start, end cpu.TimesStat) float64 {
	return Total(end) - Total(start)
}

func IdleBetween(start, end cpu.TimesStat) float64 {
	return Idle(end) - Idle(start)
}
