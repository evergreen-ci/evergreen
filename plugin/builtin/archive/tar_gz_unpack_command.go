package archive

import (
	"10gen.com/mci/archive"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"os"
)

// Plugin command responsible for unpacking a tgz archive.
type TarGzUnpackCommand struct {
	// the tgz file to unpack
	Source string `mapstructure:"source"`
	// the directory that the unpacked contents should be put into
	DestDir string `mapstructure:"dest_dir"`
}

func (self *TarGzUnpackCommand) Name() string {
	return TarGzUnpackCmdName
}

// Implementation of ParseParams.
func (self *TarGzUnpackCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error parsing '%v' params: %v", self.Name(), err)
	}
	if err := self.validateParams(); err != nil {
		return fmt.Errorf("error validating '%v' params: %v", self.Name(), err)
	}
	return nil
}

// Make sure both source and dest dir are speciifed.
func (self *TarGzUnpackCommand) validateParams() error {

	if self.Source == "" {
		return fmt.Errorf("source cannot be blank")
	}
	if self.DestDir == "" {
		return fmt.Errorf("dest_dir cannot be blank")
	}

	return nil
}

// Implementation of Execute, to unpack the archive.
func (self *TarGzUnpackCommand) Execute(pluginLogger plugin.PluginLogger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {

	// TODO: expand params?

	errChan := make(chan error)
	go func() {
		errChan <- self.UnpackArchive()
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of targz unpack command")
		return nil
	}
}

// UnpackArchive unpacks the archive. The target archive to unpack is
// set for the command during parameter parsing.
func (self *TarGzUnpackCommand) UnpackArchive() error {

	// get a reader for the source file
	f, _, tarReader, err := archive.TarGzReader(self.Source)
	if err != nil {
		return fmt.Errorf("error opening tar file %v for reading: %v",
			self.Source, err)
	}
	defer f.Close()

	// extract the actual tarball into the destination directory
	if err := os.MkdirAll(self.DestDir, 0755); err != nil {
		return fmt.Errorf("error creating destination dir %v: %v", self.DestDir,
			err)
	}

	return archive.Extract(tarReader, self.DestDir)
}
