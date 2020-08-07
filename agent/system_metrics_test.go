package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
)

func TestCollectDiskUsage(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	diskUsageCollector := &diskUsageCollector{}

	dir, _ := os.Getwd()
	output, err := diskUsageCollector.Collect(ctx, dir)
	assert.NoError(err)

	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		docIter := iter.Document().Iterator()
		expectedKeys := []string{"total", "free", "used", "usedPercent", "inodesTotal",
			"inodesUsed", "inodesFree", "inodesUsedPercent"}

		j := 0
		for docIter.Next() {
			currKey, keyOk := docIter.Element().KeyOK()
			assert.Equal(expectedKeys[j], currKey)
			assert.True(keyOk)
			j++
		}
		i++
	}
	assert.Equal(1, i)
}

func TestCollectUptime(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	uptimeCollector := &uptimeCollector{}
	output, err := uptimeCollector.Collect(ctx)
	assert.NoError(err)

	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		docIter := iter.Document().Iterator()
		expectedKeys := []string{"uptime"}

		j := 0
		for docIter.Next() {
			currKey, keyOk := docIter.Element().KeyOK()
			assert.Equal(expectedKeys[j], currKey)
			assert.True(keyOk)
			j++
		}
		i++
	}
	assert.Equal(1, i)
}

func TestCollectProcesses(t *testing.T) {
	assert := assert.New(t)
	if runtime.GOOS == "darwin" {
		t.Skip("TODO (EVG-12736): fix (*Process).CreateTime - Process() does not work on MacOS with old version of gopsutil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	processCollector := &processCollector{}
	output, err := processCollector.Collect(ctx)

	assert.NoError(err)
	assert.NotEmpty(output)

	var processes processesWrapper
	err = json.Unmarshal(output, &processes)
	assert.NoError(err)
	assert.NotEmpty(processes)

	for _, process := range processes.Processes {
		assert.NotEmpty(process.PID)
	}
}
