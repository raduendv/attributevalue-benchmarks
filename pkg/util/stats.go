package util

import (
	"context"
	"log"
	mcpu "pkg/cpu"
	"pkg/model"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"

	"github.com/shirou/gopsutil/cpu"
)

func CollectGCStats(ctx context.Context, sender func(model.Event)) {
	log.Println("CollectGCStats starting gc collector")

	var stats debug.GCStats
	var lastGC time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second / 10):
			break
		}

		debug.ReadGCStats(&stats)

		if lastGC == stats.LastGC {
			continue
		}

		//eventNumGC := model.Event{
		//	API:            Pointer("GCStats.NumGC"),
		//	Latency:        Pointer(int(stats.NumGC)),
		//	Type:           Pointer("AttributeValue"),
		//	Service:        Pointer("DynamoDB"),
		//	ClientID:       Pointer("v1"),
		//	Timestamp:      (*model.MetricTime)(Pointer(stats.LastGC)),
		//	Version:        Pointer(1),
		//	AttemptCount:   Pointer(1),
		//	AttemptLatency: nil,
		//	StandardUnit:   Pointer(cloudwatchlogs.StandardUnitCount),
		//}

		sender(CreateEvent(
			"GCStats.NumGC",
			float64(stats.NumGC),
			stats.LastGC,
			cloudwatchlogs.StandardUnitCount,
		))

		//eventPauseTotal := model.Event{
		//	API:            Pointer("GCStats.PauseTotal"),
		//	Latency:        Pointer(int(stats.PauseTotal.Microseconds())),
		//	Type:           Pointer("AttributeValue"),
		//	Service:        Pointer("DynamoDB"),
		//	ClientID:       Pointer("v1"),
		//	Timestamp:      (*model.MetricTime)(Pointer(stats.LastGC)),
		//	Version:        Pointer(1),
		//	AttemptCount:   Pointer(1),
		//	AttemptLatency: nil,
		//	StandardUnit:   Pointer(cloudwatchlogs.StandardUnitMicroseconds),
		//}

		sender(CreateEvent(
			"GCStats.PauseTotal",
			float64(stats.PauseTotal.Microseconds()),
			stats.LastGC,
			cloudwatchlogs.StandardUnitMicroseconds,
		))

		lastGC = stats.LastGC
	}
}

func CollectMemoryStats(ctx context.Context, sender func(model.Event)) {
	log.Println("CollectMemoryStats starting memory collector")

	var stats runtime.MemStats
	var now time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second / 10):
			break
		}

		runtime.ReadMemStats(&stats)
		now = time.Now()

		sender(CreateEvent("Memory.Alloc", float64(stats.Alloc), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.TotalAlloc", float64(stats.TotalAlloc), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.Sys", float64(stats.Sys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.Lookups", float64(stats.Lookups), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.Mallocs", float64(stats.Mallocs), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.Frees", float64(stats.Frees), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.HeapAlloc", float64(stats.HeapAlloc), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.HeapSys", float64(stats.HeapSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.HeapIdle", float64(stats.HeapIdle), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.HeapInuse", float64(stats.HeapInuse), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.HeapReleased", float64(stats.HeapReleased), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.HeapObjects", float64(stats.HeapObjects), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.StackInuse", float64(stats.StackInuse), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.StackSys", float64(stats.StackSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.MSpanInuse", float64(stats.MSpanInuse), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.MSpanSys", float64(stats.MSpanSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.MCacheInuse", float64(stats.MCacheInuse), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.MCacheSys", float64(stats.MCacheSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.BuckHashSys", float64(stats.BuckHashSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.GCSys", float64(stats.GCSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.OtherSys", float64(stats.OtherSys), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.NextGC", float64(stats.NextGC), now, cloudwatchlogs.StandardUnitBytes))
		sender(CreateEvent("Memory.LastGCUs", float64(stats.LastGC/1000), time.Unix(0, int64(stats.LastGC)), cloudwatchlogs.StandardUnitMicroseconds))
		sender(CreateEvent("Memory.PauseTotalUs", float64(stats.PauseTotalNs/1000), time.Unix(0, int64(stats.LastGC)), cloudwatchlogs.StandardUnitMicroseconds))
		sender(CreateEvent("Memory.PauseUs", float64(stats.PauseNs[(stats.NumGC+255)%256]/1000), time.Unix(0, int64(stats.PauseEnd[(stats.NumGC+255)%256])), cloudwatchlogs.StandardUnitMicroseconds))
		//sender(CreateEvent("Memory.PauseEndUs", float64(stats.PauseEnd[(stats.NumGC+255)%256]/1000), time.Unix(0, int64(stats.LastGC)), cloudwatchlogs.StandardUnitMicroseconds))
		sender(CreateEvent("Memory.NumGC", float64(stats.NumGC), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.NumForcedGC", float64(stats.NumForcedGC), now, cloudwatchlogs.StandardUnitCount))
		sender(CreateEvent("Memory.GCCPUFraction", stats.GCCPUFraction, now, cloudwatchlogs.StandardUnitPercent))
	}
}

func CollectCPUStats(ctx context.Context, sender func(model.Event)) {
	log.Println("CollectCPUStats starting CPU collector")

	last := time.Now()
	previousTimes, _ := cpu.Times(false)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
			break
		}

		currentTimes, _ := cpu.Times(false)

		if len(currentTimes) == 0 {
			continue
		} else if len(previousTimes) == 0 {
			previousTimes = currentTimes
			continue
		}

		total := mcpu.TotalBetween(previousTimes[0], currentTimes[0])
		idle := mcpu.IdleBetween(previousTimes[0], currentTimes[0])
		usagePercentage := mcpu.UsageBetween(previousTimes[0], currentTimes[0])

		idlePercentage := 0.0
		userPercentage := 0.0
		systemPercentage := 0.0
		iowaitPercentage := 0.0
		if total > 0 {
			idlePercentage = idle / total * 100
			userPercentage = (currentTimes[0].User - previousTimes[0].User) / total * 100
			systemPercentage = (currentTimes[0].System - previousTimes[0].System) / total * 100
			iowaitPercentage = (currentTimes[0].Iowait - previousTimes[0].Iowait) / total * 100
		}

		// updates for next run
		last = time.Now()
		previousTimes = currentTimes

		// actual sending
		sender(CreateEvent("Client.CPU.Usage", usagePercentage, last, cloudwatchlogs.StandardUnitPercent))
		sender(CreateEvent("Client.CPU.Idle", idlePercentage, last, cloudwatchlogs.StandardUnitPercent))
		sender(CreateEvent("Client.CPU.User", userPercentage, last, cloudwatchlogs.StandardUnitPercent))
		sender(CreateEvent("Client.CPU.System", systemPercentage, last, cloudwatchlogs.StandardUnitPercent))
		sender(CreateEvent("Client.CPU.IOWait", iowaitPercentage, last, cloudwatchlogs.StandardUnitPercent))
	}
}

func CollectRuntimeStats(ctx context.Context, sender func(model.Event)) {
	log.Println("CollectRuntimeStats starting runtime collector")

	var now time.Time
	var descs []metrics.Description
	var samples []metrics.Sample

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second / 10):
			break
		}

		now = time.Now()
		if tmp := metrics.All(); len(tmp) != len(descs) {
			descs = tmp
			samples = make([]metrics.Sample, len(descs))
			for c := 0; c < len(descs); c++ {
				samples[c] = metrics.Sample{
					Name: descs[c].Name,
				}
			}
		}

		metrics.Read(samples)

		for _, sample := range samples {
			nameAndUnit := GoToCloudwatchMetricNameAndUnit(sample.Name)
			metricName := sample.Name
			metricUnit := cloudwatchlogs.StandardUnitCount
			if nameAndUnit[0] != "" {
				metricName = nameAndUnit[0]
			}
			if nameAndUnit[1] != "" {
				metricUnit = nameAndUnit[1]
			}

			switch sample.Value.Kind() {
			case metrics.KindUint64:
				sender(CreateEvent(
					metricName,
					float64(sample.Value.Uint64()),
					now,
					metricUnit,
				))
			case metrics.KindFloat64:
				sender(CreateEvent(
					metricName,
					sample.Value.Float64(),
					now,
					metricUnit,
				))
			default:
				continue
			}
		}
	}
}

func GoUnitToCloudwatchUnit(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	switch s {
	case "bytes":
		return cloudwatchlogs.StandardUnitBytes
	case "calls", "classes", "cleanups", "events", "finalizers", "gc-cycle", "gc-cycles", "goroutines", "objects", "threads":
		return cloudwatchlogs.StandardUnitCount
	case "cpu-seconds", "seconds":
		return cloudwatchlogs.StandardUnitSeconds
	case "percent":
		return cloudwatchlogs.StandardUnitPercent
	}

	return cloudwatchlogs.StandardUnitCount
}

func GoToCloudwatchMetricNameAndUnit(s string) [2]string {
	sep := strings.IndexByte(s, ':')
	if sep <= 0 || sep >= len(s)-1 {
		return [2]string{}
	}
	rawUnit := strings.TrimSpace(s[sep+1:])

	metricName := strings.TrimLeft(s[:sep], "/") + "." + rawUnit
	metricName = strings.ToLower(metricName)
	metricName = strings.ReplaceAll(metricName, "/", ".")
	metricName = strings.ReplaceAll(metricName, "-", "_")
	// Include unit token to avoid collisions like frees:bytes vs frees:objects.
	rawUnit = strings.ToLower(rawUnit)

	return [2]string{metricName, GoUnitToCloudwatchUnit(rawUnit)}
}
