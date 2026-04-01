package agent

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	resourceMonitorInterval = 15 * time.Second
	cpuThresholdPercent     = 90.0
	memoryThresholdPercent  = 90.0
	// sustainedSampleCount is the number of consecutive samples above threshold
	// required to mark a resource as constrained. 20 samples = 5 minutes of sustained usage at 15s intervals.
	sustainedSampleCount = 20
)

type resourceMonitor struct {
	mu sync.Mutex

	cpuConsecutive    int
	memoryConsecutive int

	cpuConstrained    bool
	memoryConstrained bool

	peakCPUPercent    float64
	peakMemoryPercent float64

	logger grip.Journaler
}

func newResourceMonitor(logger grip.Journaler) *resourceMonitor {
	if logger == nil {
		logger = grip.NewJournaler("resource_monitor")
	}
	return &resourceMonitor{
		logger: logger,
	}
}

// start samples CPU and memory usage at regular intervals until the context is cancelled.
func (rm *resourceMonitor) start(ctx context.Context) {
	ticker := time.NewTicker(resourceMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.sample(ctx)
		}
	}
}

func (rm *resourceMonitor) sample(ctx context.Context) {
	// CPU percent is measured over a 200ms interval. This call will block,
	// so it should be kept low to avoid long delays. We expect exactly 1
	// result because we pass percpu=false.
	cpuPercents, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false)
	if err != nil {
		rm.logger.Debug(ctx, errors.Wrap(err, "sampling CPU usage"))
	} else if len(cpuPercents) > 0 {
		rm.recordCPU(cpuPercents[0])
	} else {
		rm.logger.Warning(ctx, "CPU usage sampling returned empty result")
	}

	memStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		rm.logger.Warning(ctx, errors.Wrap(err, "sampling memory usage"))
	} else if memStat != nil {
		rm.recordMemory(memStat.UsedPercent)
	}
}

func (rm *resourceMonitor) recordCPU(percent float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if percent > rm.peakCPUPercent {
		rm.peakCPUPercent = percent
	}

	if percent >= cpuThresholdPercent {
		rm.cpuConsecutive++
		if rm.cpuConsecutive >= sustainedSampleCount {
			rm.cpuConstrained = true
		}
	} else {
		rm.cpuConsecutive = 0
	}
}

func (rm *resourceMonitor) recordMemory(percent float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if percent > rm.peakMemoryPercent {
		rm.peakMemoryPercent = percent
	}

	if percent >= memoryThresholdPercent {
		rm.memoryConsecutive++
		if rm.memoryConsecutive >= sustainedSampleCount {
			rm.memoryConstrained = true
		}
	} else {
		rm.memoryConsecutive = 0
	}
}

func (rm *resourceMonitor) report() *apimodels.ResourceConstraintInfo {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.cpuConstrained && !rm.memoryConstrained {
		return nil
	}

	return &apimodels.ResourceConstraintInfo{
		CPUConstrained:    rm.cpuConstrained,
		MemoryConstrained: rm.memoryConstrained,
		PeakCPUPercent:    rm.peakCPUPercent,
		PeakMemoryPercent: rm.peakMemoryPercent,
	}
}
