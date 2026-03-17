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
	sustainedSampleCount    = 20
)

type cpuAndMemoryMonitor struct {
	mu sync.Mutex

	cpuConsecutive    int
	memoryConsecutive int

	cpuConstrained    bool
	memoryConstrained bool

	peakCPUPercent    float64
	peakMemoryPercent float64
}

func newResourceMonitor() *cpuAndMemoryMonitor {
	return &cpuAndMemoryMonitor{}
}

// start samples CPU and memory usage at regular intervals until the context is cancelled.
func (rm *cpuAndMemoryMonitor) start(ctx context.Context) {
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

func (rm *cpuAndMemoryMonitor) sample(ctx context.Context) {
	// one second is the interval that is measured over.
	cpuPercents, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil {
		grip.Debug(errors.Wrap(err, "sampling CPU usage"))
	} else if len(cpuPercents) > 0 {
		rm.recordCPU(cpuPercents[0])
	}

	memStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		grip.Debug(errors.Wrap(err, "sampling memory usage"))
	} else if memStat != nil {
		rm.recordMemory(memStat.UsedPercent)
	}
}

func (rm *cpuAndMemoryMonitor) recordCPU(percent float64) {
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

func (rm *cpuAndMemoryMonitor) recordMemory(percent float64) {
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

func (rm *cpuAndMemoryMonitor) report() *apimodels.ResourceConstraintInfo {
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
