package command

import (
	"context"
	"os"

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
		err = errors.Wrap(err, "problem finding wildcard paths")
		logger.Task().Error(err)
		return err
	}

	catcher := grip.NewBasicCatcher()
	files := []*artifact.File{}
	var segment []*artifact.File
	for idx := range c.Files {
		segment, err = readArtifactsFile(c.Files[idx])
		if err != nil {
			catcher.Add(err)
			continue
		}

		if segment == nil {
			continue
		}

		files = append(files, segment...)
	}

	if catcher.HasErrors() {
		err = errors.Wrap(catcher.Resolve(), "encountered errors reading artifact json files")
		logger.Execution().Error(err)
		return err
	}

	if len(files) == 0 {
		logger.Execution().Warning("no artifacts defined")
		return nil
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if comm.AttachFiles(ctx, td, files); err != nil {
		return errors.Wrap(err, "attach artifacts failed")
	}

	logger.Execution().Infof("'$s' attached %d resources to task", c.Name(), len(files))
	return nil
}

func readArtifactsFile(fn string) ([]*artifact.File, error) {
	file, err := os.Open(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "problem opening file '%s'", fn)
	}
	defer file.Close()

	out := []*artifact.File{}

	if err = util.ReadJSONInto(file, out); err != nil {
		return nil, errors.Wrapf(err, "problem reading JSON from file '%s'", fn)
	}

	return out, nil
}
