package jasper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPids(t *testing.T) {
	assert := assert.New(t)

	dmesg := "[11686.043647] Killed process 2603 (flasherav) total-vm:1498536kB, anon-rss:721784kB, file-rss:4228kB"
	assert.True(dmesgContainsOOMKill(dmesg))
	pid, hasPid := getPidFromDmesg(dmesg)
	assert.True(hasPid)
	assert.Equal(2603, pid)

	dmesg = "Killed process 9823, UID 0, (FlowCon.fresher) total-vm:3098244kB, anon-rss:1157280kB, file-rss:36kB"
	assert.True(dmesgContainsOOMKill(dmesg))
	pid, hasPid = getPidFromDmesg(dmesg)
	assert.True(hasPid)
	assert.Equal(9823, pid)

	log := "2018-10-03 21:55:21.478932+0000 0x16b Default 0x0 0 kernel: low swap: killing largest compressed process with pid 29670 (mongod) and size 1 MB"
	pid, hasPid = getPidFromLog(log)
	assert.True(hasPid)
	assert.Equal(29670, pid)
}
