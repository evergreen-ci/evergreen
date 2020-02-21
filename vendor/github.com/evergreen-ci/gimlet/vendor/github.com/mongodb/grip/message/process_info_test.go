package message

import (
	"os"
	"os/exec"
	"testing"

	"github.com/shirou/gopsutil/process"
	"github.com/stretchr/testify/assert"
)

func TestChildren(t *testing.T) {
	assert := assert.New(t)
	myPid := int32(os.Getpid())
	p, err := process.NewProcess(myPid)
	assert.NotNil(p)
	assert.NoError(err)
	cmd := exec.Command("sleep", "1")
	assert.NoError(cmd.Start())

	c, err := p.Children()
	assert.NotNil(c)
	assert.NoError(err)
	assert.Equal(1, len(c))
	for _, process := range c {
		assert.NotEqual(myPid, process.Pid)
	}
}
