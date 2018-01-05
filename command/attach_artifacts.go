package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type attachArtifacts struct {
	Files []string `mapstructure:"files" plugin:"expand"`
	base
}

func attachArtifactsFactory() Command   { return &attachArtifacts{} }
func (c *attachArtifacts) Name() string { return "attach.artifacts" }

func (c *attachArtifacts) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if len(c.Files) == 0 {
		return errors.Errorf("error validating params: must specify at least one "+
			"file pattern to parse: '%+v'", params)
	}
	return nil
}

func (c *attachArtifacts) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	var err error

	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		err = errors.Wrap(err, "error expanding params")
		logger.Task().Error(err)
		return err
	}

	c.Files, err = util.BuildFileList(conf.WorkDir, c.Files...)
	if err != nil {
		err = errors.Wrap(err, "problem building wildcard paths")
		logger.Task().Error(err)
		return err
	}

	if len(c.Files) == 0 {
		err = errors.New("expanded file specification had no items")
		logger.Task().Error(err)
		return err
	}

	catcher := grip.NewBasicCatcher()
	missedSegments := 0
	files := []*artifact.File{}
	var segment []*artifact.File
	for idx := range c.Files {
		segment, err = readArtifactsFile(conf.WorkDir, c.Files[idx])
		if err != nil {
			catcher.Add(err)
			continue
		}

		if segment == nil {
			missedSegments++
			continue
		}

		files = append(files, segment...)
	}

	if catcher.HasErrors() {
		err = errors.Wrap(catcher.Resolve(), "encountered errors reading artifact json files")
		logger.Task().Error(err)
		return err
	}

	if missedSegments > 0 {
		logger.Task().Noticef("encountered %d empty file definitions", missedSegments)
	}

	if len(files) == 0 {
		logger.Task().Warning("no artifacts defined")
		return nil
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if err = comm.AttachFiles(ctx, td, files); err != nil {
		return errors.Wrap(err, "attach artifacts failed")
	}

	logger.Task().Infof("'%s' attached %d resources to task", c.Name(), len(files))
	return nil
}

func readArtifactsFile(wd, fn string) ([]*artifact.File, error) {
	if !filepath.IsAbs(fn) {
		fn = filepath.Join(wd, fn)
	}

	file, err := os.Open(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "problem opening file '%s'", fn)
	}
	defer file.Close()

	out := []*artifact.File{}

	if err = util.ReadJSONInto(file, &out); err != nil {
		return nil, errors.Wrapf(err, "problem reading JSON from file '%s'", fn)
	}

	return out, nil
}
