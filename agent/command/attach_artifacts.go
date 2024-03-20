package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type attachArtifacts struct {
	// Files is a list of files, using gitignore syntax.
	Files []string `mapstructure:"files" plugin:"expand"`

	// ExactFileNames, when set to true, causes this command to treat the files array as an array of exact filenames to match,
	// rather than the default behavior, which treats files as an array of gitignore file globs.
	ExactFileNames bool `mapstructure:"exact_file_names"`

	// Prefix is an optional directory prefix to start file globbing in, relative to Evergreen's working directory.
	Prefix string `mapstructure:"prefix" plugin:"expand"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// the path specified in files does not exist. Defaults to false, which triggers errors
	// for missing files.
	Optional bool `mapstructure:"optional"`

	base
}

func attachArtifactsFactory() Command   { return &attachArtifacts{} }
func (c *attachArtifacts) Name() string { return evergreen.AttachArtifactsCommandName }

func (c *attachArtifacts) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if len(c.Files) == 0 {
		return errors.Errorf("must specify at least one file pattern to parse")
	}
	return nil
}

func (c *attachArtifacts) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	var err error

	if err = util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	if !c.ExactFileNames {
		workDir := GetWorkingDirectory(conf, c.Prefix)
		include := utility.NewGitIgnoreFileMatcher(workDir, c.Files...)
		b := utility.FileListBuilder{
			WorkingDir: workDir,
			Include:    include,
		}
		if c.Files, err = b.Build(); err != nil {
			return errors.Wrap(err, "building wildcard paths")
		}
	}

	if len(c.Files) == 0 {
		err = errors.New("expanded file specification had no items")
		if c.Optional {
			logger.Task().Error(err)
			return nil
		}
		return err
	}

	catcher := grip.NewBasicCatcher()
	missedSegments := 0
	files := []*artifact.File{}
	var segment []*artifact.File
	for idx := range c.Files {
		segment, err = readArtifactsFile(GetWorkingDirectory(conf, c.Prefix), c.Files[idx])
		if err != nil {
			if c.Optional && os.IsNotExist(errors.Cause(err)) {
				// pass;
			} else {
				catcher.Add(err)
			}
			continue
		}

		if segment == nil {
			missedSegments++
			continue
		}

		files = append(files, segment...)
	}
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "reading artifact JSON files")
	}

	if missedSegments > 0 {
		logger.Task().Noticef("Encountered %d empty file definitions.", missedSegments)
	}

	if len(files) == 0 {
		logger.Task().Warning("No artifacts defined.")
		return nil
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if err = comm.AttachFiles(ctx, td, files); err != nil {
		return errors.Wrap(err, "attach artifacts failed")
	}

	logger.Task().Infof("'%s' attached %d resources to task.", c.Name(), len(files))
	return nil
}

func readArtifactsFile(wd, fn string) ([]*artifact.File, error) {
	if !filepath.IsAbs(fn) {
		fn = filepath.Join(wd, fn)
	}

	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "file '%s' does not exist", fn)
	}

	file, err := os.Open(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file '%s'", fn)
	}
	defer file.Close()

	out := []*artifact.File{}

	if err = utility.ReadJSON(file, &out); err != nil {
		return nil, errors.Wrapf(err, "reading JSON from file '%s'", fn)
	}

	return out, nil
}
