package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type tarballExtract struct {
	// ArchivePath is the path of the tarball to extract.
	ArchivePath string `mapstructure:"path" plugin:"expand"`

	// TargetDirectory is the directory to extract the tarball to.
	TargetDirectory string `mapstructure:"destination" plugin:"expand"`

	// a list of filename blobs to exclude when extracting
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func tarballExtractFactory() Command   { return &tarballExtract{} }
func (e *tarballExtract) Name() string { return "archive.targz_extract" }

func (e *tarballExtract) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, e); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(e.ArchivePath == "", "archive path must be specified")
	catcher.NewWhen(e.TargetDirectory == "", "target directory must be specified")

	return catcher.Resolve()
}

func (e *tarballExtract) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(e, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	destinationPath := GetWorkingDirectory(conf, e.TargetDirectory)
	archivePath := GetWorkingDirectory(conf, e.ArchivePath)

	archive, err := os.Open(archivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.Errorf("archive '%s' does not exist", archivePath)
		}
		return errors.Wrapf(err, "reading file '%s'", archivePath)
	}
	defer func() {
		logger.Task().Notice(errors.Wrapf(archive.Close(), "closing file '%s'", archivePath))
	}()

	if err := extractTarball(ctx, archive, destinationPath, e.ExcludeFiles); err != nil {
		return errors.Wrapf(err, "extracting file '%s'", archivePath)
	}

	return nil
}
