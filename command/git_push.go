package command

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// gitMerge is a command that commits tracked changes and pushes the resulting commit to a remote repository
type gitPush struct {
	// The root directory (locally) that the code is checked out into.
	// Must be a valid non-blank directory name.
	Directory      string `yaml:"directory" plugin:"expand"`
	DryRun         bool   `yaml:"dry_run" mapstructure:"dry_run"`
	CommitterName  string `yaml:"committer_name" mapstructure:"committer_name"`
	CommitterEmail string `yaml:"committer_email" mapstructure:"committer_email"`
	Token          string `yaml:"token" plugin:"expand" mapstructure:"token"`

	base
}

func gitPushFactory() Command   { return &gitPush{} }
func (c *gitPush) Name() string { return "git.push" }

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
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "can't apply expansions")
	}

	p, err := comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
	if err != nil {
		return errors.Wrap(err, "Failed to get patch")
	}

	checkoutCommand := fmt.Sprintf("git checkout %s", conf.ProjectRef.Branch)
	logger.Execution().Debugf("git checkout command %s", checkoutCommand)
	jpm := c.JasperManager()
	cmd := jpm.CreateCommand(ctx).Directory(filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory))).Append(checkoutCommand).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())
	if err = cmd.Run(ctx); err != nil {
		return errors.Wrapf(err, "can't checkout '%s' branch", conf.ProjectRef.Branch)
	}

	// fail the merge if HEAD has moved
	logger.Execution().Info("Checking HEAD")
	headSHA, err := c.revParse(ctx, conf, logger, "HEAD")
	if err != nil {
		return errors.Wrap(err, "can't get SHA for HEAD")
	}
	if p.Githash != headSHA {
		return errors.Errorf("tip of branch '%s' has moved. Expecting '%s', but found '%s'", conf.ProjectRef.Branch, p.Githash, headSHA)
	}

	// get commit information
	taskData := client.TaskData{
		ID:     conf.Task.Id,
		Secret: conf.Task.Secret,
	}
	u, err := comm.GetUserAuthorInfo(ctx, taskData, p.Author)
	if err != nil {
		return errors.Wrapf(err, "can't get author information for user '%s'", p.Author)
	}

	_, projectToken, err := getProjectMethodAndToken(c.Token, conf.Expansions.Get(evergreen.GlobalGitHubTokenExpansion), conf.Distro.CloneMethod)
	if err != nil {
		return errors.Wrap(err, "failed to get token")
	}

	params := pushParams{
		authorName:  restModel.FromAPIString(u.DisplayName),
		authorEmail: restModel.FromAPIString(u.Email),
		description: p.Description,
		token:       projectToken,
	}

	// push module patches
	for _, modulePatch := range p.Patches {
		if modulePatch.ModuleName == "" {
			continue
		}

		if len(modulePatch.PatchSet.Summary) == 0 {
			logger.Execution().Infof("Skipping empty patch for module '%s' on patch ID '%s'", modulePatch.ModuleName, p.Id.Hex())
			continue
		}

		var module *model.Module
		module, err = conf.Project.GetModuleByName(modulePatch.ModuleName)
		if err != nil {
			logger.Execution().Errorf("No module found for %s", modulePatch.ModuleName)
			continue
		}
		moduleBase := filepath.Join(module.Prefix, module.Name)

		checkoutCommand = fmt.Sprintf("git checkout %s", module.Branch)
		logger.Execution().Debugf("git checkout command: %s", checkoutCommand)
		cmd := jpm.CreateCommand(ctx).Directory(filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory, moduleBase))).Append(checkoutCommand).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())
		if err = cmd.Run(ctx); err != nil {
			return errors.Wrapf(err, "can't checkout '%s' branch", module.Branch)
		}

		logger.Execution().Infof("Pushing patch for module %s", module.Name)
		params.directory = filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory, moduleBase))
		params.branch = module.Branch

		// File list
		params.files = make([]string, 0, len(modulePatch.PatchSet.Summary))
		for _, summary := range modulePatch.PatchSet.Summary {
			params.files = append(params.files, summary.Name)
		}

		if err = c.pushPatch(ctx, logger, params); err != nil {
			return errors.Wrap(err, "can't push module patch")
		}
	}

	// Push main patch
	for _, modulePatch := range p.Patches {
		if modulePatch.ModuleName != "" {
			continue
		}

		if len(modulePatch.PatchSet.Summary) == 0 {
			logger.Execution().Infof("Skipping empty main patch on patch id '%s'", p.Id.Hex())
			continue
		}

		// File list
		params.files = make([]string, 0, len(modulePatch.PatchSet.Summary))
		for _, summary := range modulePatch.PatchSet.Summary {
			params.files = append(params.files, summary.Name)
		}

		logger.Execution().Info("Pushing patch")
		params.directory = filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory))
		params.branch = conf.ProjectRef.Branch
		if err = c.pushPatch(ctx, logger, params); err != nil {
			return errors.Wrap(err, "can't push patch")
		}
	}

	return nil
}

type pushParams struct {
	token       string
	directory   string
	authorName  string
	authorEmail string
	description string
	branch      string
	files       []string
}

func (c *gitPush) pushPatch(ctx context.Context, logger client.LoggerProducer, p pushParams) error {
	commands := []string{}
	for _, file := range p.files {
		commands = append(commands, fmt.Sprintf(`git add "%s"`, file))
		logger.Execution().Debugf(`git add "%s"`, file)
	}

	jpm := c.JasperManager()
	cmd := jpm.CreateCommand(ctx).Directory(p.directory).Append(commands...).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())
	if err := cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "can't add files")
	}

	author := fmt.Sprintf("%s <%s>", p.authorName, p.authorEmail)
	commitCommand := fmt.Sprintf("git "+
		`-c "user.name=%s" `+
		`-c "user.email=%s" `+
		`commit --file - `+
		`--author="%s"`,
		c.CommitterName, c.CommitterEmail, author)
	logger.Execution().Debugf("git commit command: %s", commitCommand)
	cmd = jpm.CreateCommand(ctx).Directory(p.directory).Append(commitCommand).SetInput(bytes.NewBufferString(p.description)).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())
	if err := cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "can't create commit from files")
	}

	if !c.DryRun {
		stdErr := noopWriteCloser{&bytes.Buffer{}}
		pushCommand := fmt.Sprintf("git push origin %s", p.branch)
		logger.Execution().Debugf("git push command: %s", pushCommand)
		cmd = jpm.CreateCommand(ctx).Directory(p.directory).Append(pushCommand).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr)
		if err := cmd.Run(ctx); err != nil {
			return errors.Wrap(err, "can't add files")
		}

		errorOutput := stdErr.String()
		if errorOutput != "" && p.token != "" {
			errorOutput = strings.Replace(errorOutput, p.token, "[redacted oauth token]", -1)
			logger.Execution().Error(errorOutput)
		}
	}

	return nil
}

func (c *gitPush) revParse(ctx context.Context, conf *model.TaskConfig, logger client.LoggerProducer, ref string) (string, error) {
	stdout := noopWriteCloser{
		&bytes.Buffer{},
	}
	jpm := c.JasperManager()

	revParseCommand := fmt.Sprintf("git rev-parse %s", ref)
	logger.Execution().Debugf("git rev-parse command: %s", revParseCommand)
	cmd := jpm.CreateCommand(ctx).Directory(filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory))).Append(revParseCommand).SetOutputWriter(stdout).
		SetErrorSender(level.Error, logger.Task().GetSender())

	if err := cmd.Run(ctx); err != nil {
		return "", errors.WithStack(err)
	}

	return strings.TrimSpace(stdout.String()), nil
}
