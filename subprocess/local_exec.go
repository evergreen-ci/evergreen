// Local Exec
//
// The "local exec" implementation of the command interface is roughly
// equvalent to the LocalCommand interface; however, it does not
// enforce the use of any shell and
//
package subprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mongodb/grip"
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

func NewLocalExec(binary string, args []string, env map[string]string, workingdir string, enheritEnv bool) (Command, error) {
	if binary == "" {
		return nil, errors.New("must specify a command")
	}

	c := &localExec{
		binary: binary,
		args:   args,
	}

	if enheritEnv {
		c.env = os.Environ()
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
	}

	return c, nil
}

func (c *localExec) Run(ctx context.Context) error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if err := c.Start(); err != nil {
		return errors.WithStack(err)
	}

	errChan := make(chan error)
	go func() {
		select {
		case errChan <- c.cmd.Wait():
		case <-ctx.Done():
		}
	}()

	select {
	case <-ctx.Done():
		err = c.cmd.Process.Kill()
		return errors.Wrapf(err,
			"operation '%s %s' was canceled and terminated.",
			c.binary, strings.Join(c.args, " "))
	case err = <-errChan:
		return errors.WithStack(err)
	}
}

func (c *localExec) Wait() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.cmd.Wait()
}

func (c *localExec) Start() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cmd = exec.CommandContext(c.binary, c.args...) // nolint
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

func (c *localExec) GetPid() error {
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
