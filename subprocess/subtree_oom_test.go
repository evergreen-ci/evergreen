package subprocess

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPids(t *testing.T) {
	assert := assert.New(t)

	mockLog := `2018-10-03 21:55:21.478932+0000 0x16b Default 0x0 0 kernel: low swap: killing largest compressed process with pid 29670 (mongod) and size 1 MB 
2019-02-11 10:36:59.140566-0500 0x44a44d   Default     0x0                  50     0    mediaremoted: (MediaRemote) [com.apple.amp.mediaremote:MediaRemote] Warning: Unknown device network ID
2019-02-11 10:36:59.993992-0500 0x448817   Default     0x0                  571    3    Finder: (Sharing) [com.apple.sharing:Browser] SFBrowserCallBack (node = <SFNode 0x6000000e6180>{domain = Network})`
	logs, err := syscall.ByteSliceFromString(mockLog)
	assert.NoError(err)

	mockDmesg := `[11686.043631] [ 2938]   999  2938      553        0   0       0             0 gnome-pty-helpe
[11686.043636] [ 2939]   999  2939     1814      406   0       0             0 bash
[11686.043641] Out of memory: Kill process 2603 (flasherav) score 761 or sacrifice child
[11686.043647] Killed process 2603 (flasherav) total-vm:1498536kB, anon-rss:721784kB, file-rss:4228kB`
	dmesg, err := syscall.ByteSliceFromString(mockDmesg)
	assert.NoError(err)

	pids := getPidsDarwin(logs)
	assert.Len(pids, 1)

	pids = getPidsLinux(dmesg)
	assert.Len(pids, 1)
}
