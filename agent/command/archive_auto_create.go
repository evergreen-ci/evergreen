package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mholt/archiver/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type autoArchiveCreate struct {
	// Target is the archive file that will be created. The target file's
	// extension will determine the archiving format to use.
	Target string `mapstructure:"target" plugin:"expand"`

	// SourceDir is the directory to compress.
	SourceDir string `mapstructure:"source_dir" plugin:"expand"`

	// Include is a list of filename blobs to include from the source directory,
	// e.g. "*.tgz", "file.txt", "test_*". If not specified, the entire source
	// directory will be used.
	Include []string `mapstructure:"include" plugin:"expand"`

	// ExcludeFiles is a list of filename blobs to exclude from the source
	// directory, e.g. "*.auto", "results.out", "ignore/**"
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func autoArchiveCreateFactory() Command { return &autoArchiveCreate{} }

func (c *autoArchiveCreate) Name() string { return "archive.auto_pack" }

func (c *autoArchiveCreate) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(c.Target == "", "target cannot be blank")
	catcher.NewWhen(c.SourceDir == "", "source directory cannot be blank")
	catcher.NewWhen(len(c.ExcludeFiles) > 0 && len(c.Include) == 0, "if specifying files to exclude, must also specify files to include")

	return catcher.Resolve()
}

func (c *autoArchiveCreate) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	c.SourceDir = GetWorkingDirectory(conf, c.SourceDir)
	c.Target = GetWorkingDirectory(conf, c.Target)

	var filenames []string
	if len(c.Include) == 0 && len(c.ExcludeFiles) == 0 {
		// If using the whole source directory, skip the unnecessary search for
		// matching files.
		filenames = []string{c.SourceDir}
	} else {
		files, _, err := findContentsToArchive(ctx, c.SourceDir, c.Include, c.ExcludeFiles)
		if err != nil {
			return errors.Wrap(err, "finding files to archive")
		}

		filenames = make([]string, len(files))
		for idx := range files {
			filenames[idx] = files[idx].path
		}
	}

	if err := archiver.Archive(filenames, c.Target); err != nil {
		return errors.Wrapf(err, "constructing auto archive '%s'", c.Target)
	}

	logger.Task().Info(message.Fields{
		"target":    c.Target,
		"num_files": len(filenames),
		"message":   "successfully created archive",
	})

	return nil
}
