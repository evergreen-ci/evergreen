package subprocess

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	"github.com/mongodb/grip"
)

type LocalExec struct {
	binary           string
	args             []string
	workingDirectory string
	env              []string
	output           OutputOptions
	cmd              *exec.Cmd
	mutex            sync.RWMutex
}

func NewLocalExec(binary string, args []string, env map[string]string) (*LocalExec, error) {
	if binary == "" {
		return nil, errors.New("must specify a command")
	}

	c := &LocalExec{
		binary: binary,
		args:   args,
	}

	for k, v := range env {
		c.env = append(c.env, fmt.Sprintf("%s=%s", k, v))
	}

	return c, nil
}

func (c *LocalExec) Run(ctx context.Context) error {

}
func (c *LocalExec) Wait() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.cmd.Wait()
}

func (c *LocalExec) Start() error {}
func (c *LocalExec) Stop() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}

	grip.Warning("Trying to stop exec Cmd / Process was nil")
	return nil
}

func (c *LocalExec) GetPid() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.cmd == nil {
		return -1
	}

	return c.cmd.Process.Pid
}

func (c *LocalExec) SetOutput(opts OutputOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.WithStack(err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.output = opts

	return nil

}
