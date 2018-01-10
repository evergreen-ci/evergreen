// Local Exec
//
// The "local exec" implementation of the command interface is roughly
// equvalent to the LocalCommand interface; however, it does not
// enforce the use of any shell and
//
package subprocess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type localExec struct {
	binary           string
	args             []string
	workingDirectory string
	env              []string
	output           OutputOptions
	cmd              *exec.Cmd
	mutex            sync.RWMutex
}

func NewLocalExec(binary string, args []string, env map[string]string, workingdir string) (Command, error) {
	if binary == "" {
		return nil, errors.New("must specify a command")
	}

	c := &localExec{
		binary: binary,
		args:   args,
		env:    os.Environ(),
	}

	for k, v := range env {
		c.env = append(c.env, fmt.Sprintf("%s=%s", k, v))
	}

	if workingdir == "" {
		var err error
		c.workingDirectory, err = os.Getwd()
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		if stat, err := os.Stat(workingdir); os.IsNotExist(err) {
			return nil, errors.Errorf("could not use non-extant %s as working directory", workingdir)
		} else if !stat.IsDir() {
			return nil, errors.Errorf("could not use file as working directory")
		}

		c.workingDirectory = workingdir
	}

	return c, nil
}

func (c *localExec) Run(ctx context.Context) error {
	if err := c.Start(ctx); err != nil {
		return errors.WithStack(err)
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return errors.WithStack(c.cmd.Wait())
}

func (c *localExec) Wait() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.cmd.Wait()
}

func (c *localExec) Start(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cmd = exec.CommandContext(ctx, c.binary, c.args...) // nolint
	c.cmd.Dir = c.workingDirectory
	c.cmd.Env = c.env

	c.cmd.Stderr = c.output.GetError()
	c.cmd.Stdout = c.output.GetOutput()

	return c.cmd.Start()
}
func (c *localExec) Stop() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}

	grip.Warning("Trying to stop exec Cmd / Process was nil")
	return nil
}

func (c *localExec) GetPid() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.cmd == nil {
		return -1
	}

	return c.cmd.Process.Pid
}

func (c *localExec) SetOutput(opts OutputOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.WithStack(err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.output = opts

	return nil

}
