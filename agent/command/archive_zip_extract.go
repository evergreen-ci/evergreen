package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mholt/archiver/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type zipExtract struct {
	ArchivePath string `mapstructure:"path" plugin:"expand"`

	TargetDirectory string `mapstructure:"destination" plugin:"expand"`

	base
}

func zipExtractFactory() Command   { return &zipExtract{} }
func (e *zipExtract) Name() string { return "archive.zip_extract" }
func (e *zipExtract) ParseParams(params map[string]any) error {
	if err := mapstructure.Decode(params, e); err != nil {
		return errors.Wrapf(err, "decoding mapstructure params")
	}

	if e.ArchivePath == "" {
		return errors.New("archive path must be specified")
	}

	return nil
}

func (e *zipExtract) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(e, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(e.TargetDirectory) {
		e.TargetDirectory = GetWorkingDirectory(conf, e.TargetDirectory)
	}

	if !filepath.IsAbs(e.ArchivePath) {
		e.ArchivePath = GetWorkingDirectory(conf, e.ArchivePath)
	}

	if _, err := os.Stat(e.ArchivePath); os.IsNotExist(err) {
		return errors.Errorf("archive '%s' does not exist", e.ArchivePath)
	}

	if err := archiver.NewZip().Unarchive(e.ArchivePath, e.TargetDirectory); err != nil {
		return errors.Wrapf(err, "extracting archive '%s' to directory '%s'", e.ArchivePath, e.TargetDirectory)
	}

	return nil
}
