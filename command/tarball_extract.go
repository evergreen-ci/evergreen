package command

import (
	"os"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Plugin command responsible for unpacking a tgz archive.
type tarballExtract struct {
	// the tgz file to unpack
	Source string `mapstructure:"source" plugin:"expand"`
	// the directory that the unpacked contents should be put into
	DestDir string `mapstructure:"dest_dir" plugin:"expand"`
}

func tarballExtractFactory() Command     { return &tarballExtract{} }
func (c *tarballExtract) Name() string   { return "targz_unpack" }
func (c *tarballExtract) Plugin() string { return "archive" }

// Implementation of ParseParams.
func (c *tarballExtract) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error parsing '%v' params", c.Name())
	}
	if err := c.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating '%v' params", c.Name())
	}
	return nil
}

// Make sure both source and dest dir are speciifed.
func (c *tarballExtract) validateParams() error {

	if c.Source == "" {
		return errors.New("source cannot be blank")
	}
	if c.DestDir == "" {
		return errors.New("dest_dir cannot be blank")
	}

	return nil
}

// Implementation of Execute, to unpack the archive.
func (c *tarballExtract) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding params")
	}

	errChan := make(chan error)
	go func() {
		errChan <- c.unpackArchive(ctx)
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of targz unpack command")
		return nil
	}
}

// UnpackArchive unpacks the archive. The target archive to unpack is
// set for the command during parameter parsing.
func (c *tarballExtract) unpackArchive(ctx context.Context) error {

	// get a reader for the source file
	f, _, tarReader, err := TarGzReader(c.Source)
	if err != nil {
		return errors.Wrapf(err, "error opening tar file %v for reading", c.Source)
	}
	defer f.Close()

	// extract the actual tarball into the destination directory
	if err := os.MkdirAll(c.DestDir, 0755); err != nil {
		return errors.Wrapf(err, "error creating destination dir %v", c.DestDir)
	}

	return errors.WithStack(util.Extract(ctx, tarReader, c.DestDir))
}
