// +build darwin

package jasper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPIDFromLog(t *testing.T) {
	log := "2018-10-03 21:55:21.478932+0000 0x16b Default 0x0 0 kernel: low swap: killing largest compressed process with pid 29670 (mongod) and size 1 MB"
	pid, hasPID := getPIDFromLog(log)
	assert.True(t, hasPID)
	assert.Equal(t, 29670, pid)
}
