package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/k0kubun/pp"
	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
)

func TestCollectDiskUsage(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	diskUsageCollector := &diskUsageCollector{}
	path, _ := filepath.Abs("/")
	output, err := diskUsageCollector.Collect(ctx, path)
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
	ctx := context.TODO()
	processCollector := &processCollector{}
	output, err := processCollector.Collect(ctx)

	var processes ProcessesWrapper
	json.Unmarshal(output, &processes)

	pp.Println(processes)

	assert.NoError(err)
	assert.NotEmpty(output)
}
