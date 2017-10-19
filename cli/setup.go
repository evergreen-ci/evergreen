package cli

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// setupTimeout is how long the agent will wait before timing out running
// the setup script.
const (
	setupTimeout            = 2 * time.Minute
	defaultWorkingDirectory = "~"
)

// SetupCommand runs setup.sh to set up a host.
type SetupCommand struct {
	WorkingDirectory string `long:"working_directory" default:"" description:"working directory"`
	SetupAsSudo      bool   `long:"setup_as_sudo" description:"run setup script as sudo"`
}

// Execute runs a script called "setup.sh" in the host's working directory.
func (c *SetupCommand) Execute(_ []string) error {
	if c.WorkingDirectory == "" {
		usr, err := user.Current()
		if err != nil {
			return err
		}
		c.WorkingDirectory = usr.HomeDir
	}

	out, err := c.runSetupScript(context.TODO())
	if err != nil {
		return errors.Wrap(err, out)
	}
	return nil
}

func (c *SetupCommand) runSetupScript(ctx context.Context) (string, error) {
	script := filepath.Join(c.WorkingDirectory, evergreen.SetupScriptName)
	if _, err := os.Stat(script); os.IsNotExist(err) {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()

	catcher := grip.NewSimpleCatcher()

	chmod := c.getChmodCommandWithSudo(ctx, script)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	cmd := c.getShCommandWithSudo(ctx, script)
	out, err = cmd.CombinedOutput()
	catcher.Add(err)

	if err := os.Remove(script); err != nil {
		catcher.Add(err)
	}

	return string(out), catcher.Resolve()
}

func (c *SetupCommand) getShCommandWithSudo(ctx context.Context, script string) *exec.Cmd {
	if c.SetupAsSudo {
		return exec.CommandContext(ctx, "sudo", "sh", script)
	}
	return exec.CommandContext(ctx, "sh", script)
}

func (c *SetupCommand) getChmodCommandWithSudo(ctx context.Context, script string) *exec.Cmd {
	args := []string{}
	if c.SetupAsSudo {
		args = append(args, "sudo")
	}
	args = append(args, "chmod", "+x", script)
	return exec.CommandContext(ctx, args[0], args[1:]...)
}
