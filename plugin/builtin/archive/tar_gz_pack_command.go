package archive

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/archive"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

// Plugin command responsible for creating a tgz archive.
type TarGzPackCommand struct {
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
}

func (self *TarGzPackCommand) Name() string {
	return TarGzPackCmdName
}

func (self *TarGzPackCommand) Plugin() string {
	return ArchivePluginName
}

// ParseParams reads in the given parameters for the command.
func (self *TarGzPackCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return errors.Wrapf(err, "error parsing '%v' params", self.Name())
	}
	if err := self.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating '%v' params", self.Name())
	}
	return nil
}

// Make sure a target and source dir are set, and files are specified to be
// included.
func (self *TarGzPackCommand) validateParams() error {
	if self.Target == "" {
		return errors.New("target cannot be blank")
	}
	if self.SourceDir == "" {
		return errors.New("source_dir cannot be blank")
	}
	if len(self.Include) == 0 {
		return errors.New("include cannot be empty")
	}

	return nil
}

// Execute builds the archive.
func (self *TarGzPackCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {

	if err := plugin.ExpandValues(self, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding params")
	}

	// if the source dir is a relative path, join it to the working dir
	if !filepath.IsAbs(self.SourceDir) {
		self.SourceDir = filepath.Join(conf.WorkDir, self.SourceDir)
	}

	// if the target is a relative path, join it to the working dir
	if !filepath.IsAbs(self.Target) {
		self.Target = filepath.Join(conf.WorkDir, self.Target)
	}

	errChan := make(chan error)
	filesArchived := -1
	go func() {
		var err error
		filesArchived, err = self.BuildArchive(pluginLogger)
		errChan <- errors.WithStack(err)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return errors.WithStack(err)
		}
		if filesArchived == 0 {
			deleteErr := os.Remove(self.Target)
			if deleteErr != nil {
				pluginLogger.LogExecution(slogger.INFO, "Error deleting empty archive: %v", deleteErr)
			}
		}
		return nil
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of targz pack command")
		return nil
	}

}

// since archive.BuildArchive takes in a slogger.Logger
type agentAppender struct {
	pluginLogger plugin.Logger
}

// satisfy the slogger.Appender interface
func (self *agentAppender) Append(log *slogger.Log) error {
	self.pluginLogger.LogExecution(log.Level, slogger.FormatLog(log))
	return nil
}

// Build the archive.
// Returns the number of files included in the archive (0 means empty archive).
func (self *TarGzPackCommand) BuildArchive(pluginLogger plugin.Logger) (int, error) {
	// create a logger to pass into the BuildArchive command
	appender := &agentAppender{
		pluginLogger: pluginLogger,
	}

	log := &slogger.Logger{
		Name:      "",
		Appenders: []send.Sender{slogger.WrapAppender(appender)},
	}

	// create a targz writer for the target file
	f, gz, tarWriter, err := archive.TarGzWriter(self.Target)
	if err != nil {
		return -1, errors.Wrapf(err, "error opening target archive file %s", self.Target)
	}
	defer func() {
		grip.CatchError(tarWriter.Close())
		grip.CatchError(gz.Close())
		grip.CatchError(f.Close())
	}()

	// Build the archive
	out, err := archive.BuildArchive(tarWriter, self.SourceDir, self.Include,
		self.ExcludeFiles, log)
	return out, errors.WithStack(err)
}
