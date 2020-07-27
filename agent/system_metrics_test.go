package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/k0kubun/pp"
	"github.com/mongodb/ftdc"
	"github.com/stretchr/testify/assert"
)

func TestCollect(t *testing.T) {
	assert := assert.New(t)
	ctx := context.TODO()
	diskUsageCollector := &DiskUsageCollector{}
	output, err := diskUsageCollector.Collect(ctx)
	assert.NoError(err)

	//need to build actual test for expected results - right now it returns 1?
	iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
	i := 0
	for iter.Next() {
		i++
	}
	pp.Printf("i value type: %T\n", i)

	assert.Equal(100, i) // test fails on this line
}
