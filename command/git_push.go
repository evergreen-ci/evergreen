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
		return errors.Errorf("error parsing '%s' params: value for directory "+
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

	checkoutCommand := []string{"git", "checkout", conf.ProjectRef.Branch}
	logger.Execution().Debugf("git checkout command %s", strings.Join(checkoutCommand, ", "))
	if err = c.runCommand(ctx, checkoutCommand, c.Directory, logger); err != nil {
		return errors.Wrapf(err, "can't checkout '%s' branch", conf.ProjectRef.Branch)
	}

	// fail the merge if HEAD has moved
	logger.Execution().Info("Checking HEAD")
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

	logger.Execution().Info("Pushing patch")
	params := pushParams{
		directory:   c.Directory,
		authorName:  restModel.FromAPIString(u.DisplayName),
		authorEmail: restModel.FromAPIString(u.Email),
		description: p.Description,
		branch:      conf.ProjectRef.Branch,
	}
	if err = c.pushPatch(ctx, logger, params); err != nil {
		return errors.Wrap(err, "can't push patch")
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

		checkoutCommand = []string{"git", "checkout", module.Branch}
		logger.Execution().Debugf("git checkout command %s", strings.Join(checkoutCommand, ", "))
		if err = c.runCommand(ctx, checkoutCommand, moduleBase, logger); err != nil {
			return errors.Wrapf(err, "can't checkout '%s' branch", module.Branch)
		}

		logger.Execution().Infof("Pushing patch for module %s", module.Name)
		params.directory = moduleBase
		params.branch = module.Branch
		if err = c.pushPatch(ctx, logger, params); err != nil {
			return errors.Wrap(err, "can't push module patch")
		}
	}

	return nil
}

type pushParams struct {
	directory   string
	authorName  string
	authorEmail string
	description string
	branch      string
}

func (c *gitPush) pushPatch(ctx context.Context, logger client.LoggerProducer, p pushParams) error {
	addCommand := []string{"git", "add", "-A"}
	logger.Execution().Debugf("git add command: %s", strings.Join(addCommand, ", "))
	if err := c.runCommand(ctx, addCommand, p.directory, logger); err != nil {
		return errors.Wrap(err, "can't add changes to git index")
	}

	author := fmt.Sprintf("%s <%s>", p.authorName, p.authorEmail)
	commitCommand := []string{
		"git",
		"-c",
		`"user.name=Evergreen Agent"`,
		"-c",
		`"user.email=no-reply@evergreen.mongodb.com"`,
		"commit",
		"-m",
		p.description,
		fmt.Sprintf("--author=%s", author),
	}
	logger.Execution().Debugf("git commit command: %s", strings.Join(commitCommand, ", "))
	if err := c.runCommand(ctx, commitCommand, p.directory, logger); err != nil {
		return errors.Wrap(err, "can't create git commit")
	}

	pushCommand := []string{"git", "push", "origin", p.branch}
	logger.Execution().Debugf("git push command: %s", strings.Join(pushCommand, ", "))
	if err := c.runCommand(ctx, pushCommand, c.Directory, logger); err != nil {
		return errors.Wrap(err, "can't push changes to remote repository")
	}

	return nil
}

func (c *gitPush) runCommand(ctx context.Context, command []string, workDir string, logger client.LoggerProducer) error {
	jpm := c.JasperManager()
	cmd := jpm.CreateCommand(ctx).Directory(workDir).Add(command).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())

	if err := cmd.Run(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *gitPush) revParse(ctx context.Context, logger client.LoggerProducer, ref string) (string, error) {
	stdout := noopWriteCloser{
		&bytes.Buffer{},
	}
	jpm := c.JasperManager()

	revParseCommand := []string{"git", "rev-parse", ref}
	logger.Execution().Debugf("git rev-parse command: %s", strings.Join(revParseCommand, ", "))
	cmd := jpm.CreateCommand(ctx).Directory(c.Directory).Add(revParseCommand).
		SetOutputWriter(stdout)

	if err := cmd.Run(ctx); err != nil {
		return "", errors.WithStack(err)
	}

	return stdout.String(), nil
}
