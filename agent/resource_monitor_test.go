package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceMonitorRecordCPU(t *testing.T) {
	t.Run("NoConstraintBelowThreshold", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < 10; i++ {
			rm.recordCPU(50.0)
		}
		assert.Nil(t, rm.report())
	})

	t.Run("ConstraintAfterSustainedSamples", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < sustainedSampleCount-1; i++ {
			rm.recordCPU(95.0)
		}
		assert.Nil(t, rm.report())

		rm.recordCPU(95.0)
		info := rm.report()
		require.NotNil(t, info)
		assert.True(t, info.CPUConstrained)
		assert.False(t, info.MemoryConstrained)
		assert.InDelta(t, 95.0, info.PeakCPUPercent, 0.01)
	})

	t.Run("CounterResetsOnDip", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < sustainedSampleCount-1; i++ {
			rm.recordCPU(95.0)
		}
		// Dip below threshold resets the counter.
		rm.recordCPU(50.0)
		for i := 0; i < sustainedSampleCount-1; i++ {
			rm.recordCPU(95.0)
		}
		assert.Nil(t, rm.report())
	})

	t.Run("ConstraintPersistsAfterDrop", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < sustainedSampleCount; i++ {
			rm.recordCPU(95.0)
		}
		info := rm.report()
		require.NotNil(t, info)
		assert.True(t, info.CPUConstrained)
		for i := 0; i < 10; i++ {
			rm.recordCPU(50.0)
		}
		info = rm.report()
		require.NotNil(t, info)
		assert.True(t, info.CPUConstrained, "CPU constraint should persist even after CPU drops")
		assert.InDelta(t, 95.0, info.PeakCPUPercent, 0.01)
	})

	t.Run("PeakTracking", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		rm.recordCPU(70.0)
		rm.recordCPU(85.0)
		rm.recordCPU(60.0)

		rm.mu.Lock()
		peak := rm.peakCPUPercent
		rm.mu.Unlock()
		assert.InDelta(t, 85.0, peak, 0.01)
	})
}

func TestResourceMonitorRecordMemory(t *testing.T) {
	t.Run("NoConstraintBelowThreshold", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < 10; i++ {
			rm.recordMemory(50.0)
		}
		assert.Nil(t, rm.report())
	})

	t.Run("ConstraintAfterSustainedSamples", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < sustainedSampleCount; i++ {
			rm.recordMemory(92.0)
		}
		info := rm.report()
		require.NotNil(t, info)
		assert.False(t, info.CPUConstrained)
		assert.True(t, info.MemoryConstrained)
		assert.InDelta(t, 92.0, info.PeakMemoryPercent, 0.01)
	})

	t.Run("CounterResetsOnDip", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		for i := 0; i < sustainedSampleCount-1; i++ {
			rm.recordMemory(95.0)
		}
		rm.recordMemory(80.0)
		for i := 0; i < sustainedSampleCount-1; i++ {
			rm.recordMemory(95.0)
		}
		assert.Nil(t, rm.report())
	})

	t.Run("PeakTracking", func(t *testing.T) {
		rm := newResourceMonitor(nil)
		rm.recordMemory(70.0)
		rm.recordMemory(88.0)
		rm.recordMemory(60.0)

		rm.mu.Lock()
		peak := rm.peakMemoryPercent
		rm.mu.Unlock()
		assert.InDelta(t, 88.0, peak, 0.01)
	})
}

func TestResourceMonitorBothConstrained(t *testing.T) {
	rm := newResourceMonitor(nil)
	for i := 0; i < sustainedSampleCount; i++ {
		rm.recordCPU(95.0)
		rm.recordMemory(93.0)
	}
	info := rm.report()
	require.NotNil(t, info)
	assert.True(t, info.CPUConstrained)
	assert.True(t, info.MemoryConstrained)
	assert.InDelta(t, 95.0, info.PeakCPUPercent, 0.01)
	assert.InDelta(t, 93.0, info.PeakMemoryPercent, 0.01)
}
