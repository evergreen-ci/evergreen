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

type autoExtract struct {
	ArchivePath string `mapstructure:"path" plugin:"expand"`

	TargetDirectory string `mapstructure:"destination" plugin:"expand"`

	// a list of filename blobs to exclude when extracting
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func autoExtractFactory() Command   { return &autoExtract{} }
func (e *autoExtract) Name() string { return "archive.auto_extract" }
func (e *autoExtract) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, e); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if len(e.ExcludeFiles) != 0 {
		return errors.New("auto extraction does not support excluded files")
	}

	if e.ArchivePath == "" {
		return errors.New("archive path must be specified")
	}

	return nil
}

func (e *autoExtract) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(e, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(e.TargetDirectory) {
		e.TargetDirectory = getWorkingDirectory(conf, e.TargetDirectory)
	}

	if !filepath.IsAbs(e.ArchivePath) {
		e.ArchivePath = getWorkingDirectory(conf, e.ArchivePath)
	}

	if _, err := os.Stat(e.ArchivePath); os.IsNotExist(err) {
		return errors.Errorf("archive '%s' does not exist", e.ArchivePath)
	}

	if err := archiver.Unarchive(e.ArchivePath, e.TargetDirectory); err != nil {
		return errors.Wrapf(err, "extracting archive '%s' to '%s'", e.ArchivePath, e.TargetDirectory)
	}

	return nil
}
