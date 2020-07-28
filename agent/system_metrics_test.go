package agent

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
)

func TestCollectDiskUsage(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	diskUsageCollector := &DiskUsageCollector{}
	output, err := diskUsageCollector.Collect(ctx)
	assert.NoError(err)

	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		i++
		fmt.Println()
		fmt.Println(iter.Document())
	}
	assert.Equal(1, i)
}

func TestCollectUptime(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	uptimeCollector := &UptimeCollector{}
	output, err := uptimeCollector.Collect(ctx)
	assert.NoError(err)

	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		i++
		fmt.Println()
		fmt.Println(iter.Document().String())
	}
	assert.Equal(1, i)
}

func TestCollectProcesses(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	processCollector := &ProcessCollector{}
	output, err := processCollector.Collect(ctx)
	assert.NoError(err)

	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		i++
		fmt.Println()
		fmt.Println(iter.Document())
	}
	assert.Equal(10, i)
}
