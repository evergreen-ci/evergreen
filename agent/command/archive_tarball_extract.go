package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type tarballExtract struct {
	ArchivePath string `mapstructure:"path" plugin:"expand"`

	TargetDirectory string `mapstructure:"destination" plugin:"expand"`

	// a list of filename blobs to exclude when extracting
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func tarballExtractFactory() Command   { return &tarballExtract{} }
func (e *tarballExtract) Name() string { return "archive.targz_extract" }
func (e *tarballExtract) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, e); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", e.Name())
	}

	if e.ArchivePath == "" {
		return errors.New("archive path must be specified")
	}

	return nil
}

func (e *tarballExtract) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(e, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding params")
	}

	if e.TargetDirectory == "" {
		return errors.New("must specify a target directory")
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(e.TargetDirectory) {
		e.TargetDirectory = getJoinedWithWorkDir(conf, e.TargetDirectory)
	}

	if !filepath.IsAbs(e.ArchivePath) {
		e.ArchivePath = getJoinedWithWorkDir(conf, e.ArchivePath)
	}

	if _, err := os.Stat(e.ArchivePath); os.IsNotExist(err) {
		return errors.Errorf("archive '%s' does not exist", e.ArchivePath)
	}

	archive, err := os.Open(e.ArchivePath)
	if err != nil {
		return errors.Wrapf(err, "problem reading file '%s'", e.ArchivePath)
	}

	defer func() {
		logger.Task().Notice(message.WrapError(archive.Close(),
			message.NewFormatted("problem closing '%s'", e.ArchivePath)))
	}()

	if err := agentutil.ExtractTarball(ctx, archive, e.TargetDirectory, e.ExcludeFiles); err != nil {
		return errors.Wrapf(err, "problem extracting '%s'", e.ArchivePath)
	}

	return nil
}
