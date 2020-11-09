package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mholt/archiver"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type zipExtract struct {
	ArchivePath string `mapstructure:"path" plugin:"expand"`

	TargetDirectory string `mapstructure:"destination" plugin:"expand"`

	// a list of filename blobs to exclude when extracting
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func zipExtractFactory() Command   { return &zipExtract{} }
func (e *zipExtract) Name() string { return "archive.zip_extract" }
func (e *zipExtract) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, e); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", e.Name())
	}

	if len(e.ExcludeFiles) != 0 {
		return errors.New("zip extraction does not support excluded files")
	}

	if e.ArchivePath == "" {
		return errors.New("archive path must be specified")
	}

	return nil
}

func (e *zipExtract) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(e, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding params")
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(e.TargetDirectory) {
		e.TargetDirectory = filepath.Join(conf.WorkDir, e.TargetDirectory)
	}

	if !filepath.IsAbs(e.ArchivePath) {
		e.ArchivePath = filepath.Join(conf.WorkDir, e.ArchivePath)
	}

	if _, err := os.Stat(e.ArchivePath); os.IsNotExist(err) {
		return errors.Errorf("archive '%s' does not exist", e.ArchivePath)
	}

	if err := archiver.Zip.Open(e.ArchivePath, e.TargetDirectory); err != nil {
		return errors.Wrapf(err, "problem extracting archive '%s'", e.ArchivePath)
	}

	return nil
}
