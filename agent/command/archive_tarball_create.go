package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Plugin command responsible for creating a tgz archive.
type tarballCreate struct {
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

	// This is only incremented in the case of a panic.
	Attempt int

	base
}

const (
	retryError = "index > windowEnd"
	maxRetries = 1
)

func tarballCreateFactory() Command   { return &tarballCreate{} }
func (c *tarballCreate) Name() string { return "archive.targz_pack" }

// ParseParams reads in the given parameters for the command.
func (c *tarballCreate) ParseParams(params map[string]interface{}) error {
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

// Execute builds the archive.
func (c *tarballCreate) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// if the source dir is a relative path, join it to the working dir
	if !filepath.IsAbs(c.SourceDir) {
		c.SourceDir = GetWorkingDirectory(conf, c.SourceDir)
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(c.Target) {
		c.Target = GetWorkingDirectory(conf, c.Target)
	}

	errChan := make(chan error)
	filesArchived := -1
	go func() {
		defer func() {
			select {
			case errChan <- recovery.HandlePanicWithError(recover(), nil, "making archive"):
				return
			case <-ctx.Done():
				return
			}
		}()
		var err error
		filesArchived, err = c.makeArchive(ctx, logger.Execution())
		select {
		case errChan <- errors.WithStack(err):
			return
		case <-ctx.Done():
			logger.Task().Infof("Context canceled waiting for archive creation: %s.", ctx.Err())
			return
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			// we should retry if we've hit this go error
			if c.Attempt < maxRetries {
				if strings.Contains(err.Error(), retryError) {
					c.Attempt += 1
					logger.Execution().Infof("Retrying command '%s' due to error: %s.", c.Name(), err.Error())
					return c.Execute(ctx, client, logger, conf)
				}

			}
			return errors.WithStack(err)
		}
		if filesArchived == 0 {
			logger.Execution().Warning("No files were archived.")
			deleteErr := os.Remove(c.Target)
			if deleteErr != nil {
				logger.Execution().Infof("Problem deleting empty archive: %s.", deleteErr.Error())
			}
		}
		return nil
	case <-ctx.Done():
		logger.Execution().Info(message.Fields{
			"message": fmt.Sprintf("received signal to terminate execution of command '%s'", c.Name()),
			"task_id": conf.Task.Id,
		})
		return nil
	}

}

// thresholdSizeForParallelGzipCompression is the total size (in bytes) of the
// files to archive after which using parallel gzip may improve performance
// compared to regular gzip.
const thresholdSizeForParallelGzipCompression = 1024 * 1024

// Build the archive.
// Returns the number of files included in the archive (0 means empty archive).
func (c *tarballCreate) makeArchive(ctx context.Context, logger grip.Journaler) (int, error) {
	pathsToAdd, totalSize, err := findArchiveContents(ctx, c.SourceDir, c.Include, []string{})
	if err != nil {
		return 0, errors.Wrap(err, "getting archive contents")
	}

	useParallelGzip := totalSize > thresholdSizeForParallelGzipCompression
	f, gz, tarWriter, err := tarGzWriter(c.Target, useParallelGzip)
	if err != nil {
		return -1, errors.Wrapf(err, "opening target archive file '%s'", c.Target)
	}
	defer func() {
		logger.Error(tarWriter.Close())
		logger.Error(gz.Close())
		logger.Error(f.Close())
	}()

	// Build the archive
	out, err := buildArchive(ctx, tarWriter, c.SourceDir, pathsToAdd,
		c.ExcludeFiles, logger)

	return out, errors.WithStack(err)
}
