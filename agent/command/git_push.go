package command

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// gitMerge is a command that commits tracked changes and pushes the resulting commit to a remote repository
type gitPush struct {
	// The root directory (locally) that the code is checked out into.
	// Must be a valid non-blank directory name.
	Directory string `yaml:"directory" plugin:"expand"`
	DryRun    bool   `yaml:"dry_run" mapstructure:"dry_run"`
	Token     string `yaml:"token" plugin:"expand" mapstructure:"token"`

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
func (c *gitPush) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	var err error
	defer func() {
		pErr := recovery.HandlePanicWithError(recover(), nil, "unexpected error in git.push")
		status := evergreen.MergeTestSucceeded
		if err != nil || pErr != nil {
			status = evergreen.MergeTestFailed
		}
		logger.Task().Error(err)
		logger.Task().Critical(pErr)
		td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
		logger.Task().Error(comm.ConcludeMerge(ctx, conf.Task.Version, status, td))
	}()
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "can't apply expansions")
	}

	var p *patch.Patch
	p, err = comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, "")
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

	// get commit information
	var projectToken string
	_, projectToken, err = getProjectMethodAndToken(c.Token, conf.Expansions.Get(evergreen.GlobalGitHubTokenExpansion), conf.Distro.CloneMethod)
	if err != nil {
		return errors.Wrap(err, "failed to get token")
	}
	params := pushParams{token: projectToken}

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
		moduleBase := filepath.Join(expandModulePrefix(conf, module.Name, module.Prefix, logger), module.Name)

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
		if err = c.pushPatch(ctx, logger, params); err != nil {
			return errors.Wrap(err, "can't push module patch")
		}
	}

	// Push main patch
	for _, mainPatch := range p.Patches {
		if mainPatch.ModuleName != "" {
			continue
		}

		if len(mainPatch.PatchSet.Summary) == 0 {
			logger.Execution().Infof("Skipping empty main patch on patch id '%s'", p.Id.Hex())
			continue
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
	token     string
	directory string
	branch    string
}

func (c *gitPush) pushPatch(ctx context.Context, logger client.LoggerProducer, p pushParams) error {
	if c.DryRun {
		return nil
	}

	jpm := c.JasperManager()
	stdErr := noopWriteCloser{&bytes.Buffer{}}
	pushCommand := fmt.Sprintf("git push origin %s", p.branch)
	logger.Execution().Debugf("git push command: %s", pushCommand)
	cmd := jpm.CreateCommand(ctx).Directory(p.directory).Append(pushCommand).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr)
	err := cmd.Run(ctx)
	errorOutput := stdErr.String()
	if errorOutput != "" {
		if p.token != "" {
			errorOutput = strings.Replace(errorOutput, p.token, "[redacted oauth token]", -1)
		}
		logger.Execution().Error(errorOutput)
	}
	if err != nil {
		return errors.Wrap(err, "can't push to remote")
	}

	return nil
}

func (c *gitPush) revParse(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, ref string) (string, error) {
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
