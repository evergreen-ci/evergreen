// +build linux

package jasper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDmesgContainsOOMKill(t *testing.T) {
	dmesg := "[11686.043647] Killed process 2603 (flasherav) total-vm:1498536kB, anon-rss:721784kB, file-rss:4228kB"
	assert.True(t, dmesgContainsOOMKill(dmesg))
	pid, hasPID := getPIDFromDmesg(dmesg)
	assert.True(t, hasPID)
	assert.Equal(t, 2603, pid)

	dmesg = "Killed process 9823, UID 0, (FlowCon.fresher) total-vm:3098244kB, anon-rss:1157280kB, file-rss:36kB"
	assert.True(t, dmesgContainsOOMKill(dmesg))
	pid, hasPID = getPIDFromDmesg(dmesg)
	assert.True(t, hasPID)
	assert.Equal(t, 9823, pid)
}
