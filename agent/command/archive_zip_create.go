package command

import (
	"context"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mholt/archiver/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type zipArchiveCreate struct {
	// the tgz file that will be created
	Target string `mapstructure:"target" plugin:"expand"`

	// the directory to compress
	SourceDir string `mapstructure:"source_dir" plugin:"expand"`

	// a list of filename blobs to include,
	// e.g. "*.tgz", "file.txt", "test_*"
	Include []string `mapstructure:"include" plugin:"expand"`

	// a list of filename blobs to exclude,
	// e.g. "*.zip", "results.out", "ignore/**"
	ExcludeFiles []string `mapstructure:"exclude_files" plugin:"expand"`

	base
}

func zipArchiveCreateFactory() Command   { return &zipArchiveCreate{} }
func (c *zipArchiveCreate) Name() string { return "archive.zip_pack" }

func (c *zipArchiveCreate) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Target == "" {
		return errors.New("target cannot be blank")
	}

	if c.SourceDir == "" {
		return errors.New("source directory cannot be blank")
	}

	if len(c.Include) == 0 {
		return errors.New("include cannot be empty")
	}

	return nil
}

func (c *zipArchiveCreate) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// if the source dir is a relative path, join it to the working dir
	if !filepath.IsAbs(c.SourceDir) {
		c.SourceDir = getWorkingDirectory(conf, c.SourceDir)
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(c.Target) {
		c.Target = getWorkingDirectory(conf, c.Target)
	}

	files, err := agentutil.FindContentsToArchive(ctx, c.SourceDir, c.Include, c.ExcludeFiles)
	if err != nil {
		return errors.Wrap(err, "finding files to archive")
	}

	filenames := make([]string, len(files))
	for idx := range files {
		filenames[idx] = files[idx].Path
	}

	if err := archiver.NewZip().Archive(filenames, c.Target); err != nil {
		return errors.Wrapf(err, "constructing zip archive '%s'", c.Target)
	}

	logger.Task().Info(message.Fields{
		"target":    c.Target,
		"num_files": len(filenames),
		"message":   "successfully created archive",
		"format":    "zip",
	})

	return nil
}
