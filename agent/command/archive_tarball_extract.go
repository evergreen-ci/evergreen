package command

import (
	"context"
	"os"
	"path/filepath"

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

func (e *tarballExtract) ParseParams(params map[string]any) error {
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
	SetWorkdirBoundaryAttribute(ctx, conf, e.TargetDirectory, e.ArchivePath)

	// Isolated task paths must stay inside the work directory; extraction is
	// bounded to the destination.
	if err := containToWorkdir(conf, "archive path", archivePath); err != nil {
		return err
	}
	if err := containToWorkdir(conf, "destination", destinationPath); err != nil {
		return err
	}

	var (
		archive     *os.File
		extractRoot *os.Root
	)
	if conf.ContainerIsolationEnabled() {
		workRoot, err := os.OpenRoot(conf.WorkDir)
		if err != nil {
			return errors.Wrap(err, "opening work directory")
		}
		defer workRoot.Close()

		archiveRel, err := filepath.Rel(filepath.Clean(conf.WorkDir), filepath.Clean(archivePath))
		if err != nil {
			return errors.Wrapf(err, "resolving archive '%s' within the work directory", archivePath)
		}
		archive, err = workRoot.Open(filepath.ToSlash(archiveRel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errors.Errorf("archive '%s' does not exist", archivePath)
			}
			return errors.Wrapf(err, "reading file '%s'", archivePath)
		}
		defer func() {
			logger.Task().Notice(ctx, errors.Wrapf(archive.Close(), "closing file '%s'", archivePath))
		}()

		if err := os.MkdirAll(destinationPath, 0755); err != nil {
			return errors.Wrapf(err, "creating destination directory '%s'", destinationPath)
		}
		extractRoot, err = os.OpenRoot(destinationPath)
		if err != nil {
			return errors.Wrapf(err, "opening destination directory '%s'", destinationPath)
		}
		defer extractRoot.Close()
	} else {
		var err error
		archive, err = os.Open(archivePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errors.Errorf("archive '%s' does not exist", archivePath)
			}
			return errors.Wrapf(err, "reading file '%s'", archivePath)
		}
		defer func() {
			logger.Task().Notice(ctx, errors.Wrapf(archive.Close(), "closing file '%s'", archivePath))
		}()
	}

	if err := extractTarball(ctx, archive, destinationPath, e.ExcludeFiles, false, extractRoot); err != nil {
		return errors.Wrapf(err, "extracting file '%s'", archivePath)
	}

	return nil
}
