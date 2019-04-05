package command

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// gitMerge is a command that commits tracked changes and pushes the resulting commit to a remote repository
type gitPush struct {
	// The root directory (locally) that the code is checked out into.
	// Must be a valid non-blank directory name.
	Directory string `plugin:"expand"`
	base
}

func gitPushFactory() Command   { return &gitPush{} }
func (c *gitPush) Name() string { return "git.push" }

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *gitPush) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	if c.Directory == "" {
		return errors.Errorf("error parsing '%v' params: value for directory "+
			"must not be blank", c.Name())
	}

	return nil
}

// Execute gets the source code required by the project
func (c *gitPush) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	p, err := comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
	if err != nil {
		return errors.Wrap(err, "Failed to get patch")
	}

	commands := getCheckoutBranchCommands(conf.ProjectRef.Branch)
	if err = c.runCommands(ctx, commands, c.Directory, logger); err != nil {
		return errors.Wrapf(err, "can't checkout '%s' branch", conf.ProjectRef.Branch)
	}

	// fail the merge if HEAD has moved
	headSHA, err := c.revParse(ctx, logger, "HEAD")
	if err != nil {
		return errors.Wrap(err, "can't get SHA for HEAD")
	}
	if p.Githash != headSHA {
		return errors.Errorf("tip of branch '%s' has moved. Expecting '%s', but found '%s'", conf.ProjectRef.Branch, p.Githash, headSHA)
	}

	// get author information
	u, err := comm.GetUserAuthorInfo(ctx, p.Author)
	if err != nil {
		return errors.Wrapf(err, "can't get author information for user '%s'", p.Author)
	}
	displayName := restModel.FromAPIString(u.DisplayName)
	email := restModel.FromAPIString(u.Email)

	// push main patch
	commands = getCommitCommands(displayName, email, p.Description, conf.ProjectRef.Branch)
	if err = c.runCommands(ctx, commands, c.Directory, logger); err != nil {
		return errors.WithStack(err)
	}

	// push module patches
	for _, modulePatch := range p.Patches {
		if modulePatch.ModuleName == "" {
			continue
		}
		module, err := conf.Project.GetModuleByName(modulePatch.ModuleName)
		if err != nil {
			logger.Execution().Errorf("No module found for %s", modulePatch.ModuleName)
			continue
		}
		moduleBase := filepath.Join(module.Prefix, module.Name)

		commands = getCommitCommands(displayName, email, p.Description, module.Branch)
		if err = c.runCommands(ctx, commands, moduleBase, logger); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func getCommitCommands(displayName, email, message, branch string) []string {
	author := fmt.Sprintf("%s <%s>", displayName, email)
	return []string{
		"set -o xtrace",
		"set -o errexit",
		"git add -A",
		fmt.Sprintf(`git -c "user.name=Evergreen Agent" -c "user.email=no-reply@evergreen.mongodb.com" commit -m "%s" --author="%s"`, message, author),
		fmt.Sprintf(`git push origin "%s"`, branch),
	}
}

func getCheckoutBranchCommands(branch string) []string {
	return []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf(`git checkout "%s"`, branch),
	}
}

func (c *gitPush) runCommands(ctx context.Context, commands []string, workDir string, logger client.LoggerProducer) error {
	jpm := c.JasperManager()
	joinedCmds := strings.Join(commands, "\n")
	cmd := jpm.CreateCommand(ctx).Directory(workDir).Add([]string{"bash", "-c", joinedCmds}).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())

	if err := cmd.Run(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *gitPush) revParse(ctx context.Context, logger client.LoggerProducer, ref string) (string, error) {
	commands := []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf(`git rev-parse "%s"`, ref),
	}

	stdout := noopWriteCloser{
		&bytes.Buffer{},
	}
	jpm := c.JasperManager()
	joinedCmds := strings.Join(commands, "\n")
	cmd := jpm.CreateCommand(ctx).Directory(c.Directory).Add([]string{"bash", "-c", joinedCmds}).
		SetOutputWriter(stdout)

	if err := cmd.Run(ctx); err != nil {
		return "", errors.WithStack(err)
	}

	return stdout.String(), nil
}
