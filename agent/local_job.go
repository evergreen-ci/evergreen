package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/util"
	"errors"
	"github.com/10gen-labs/slogger/v1"
	"os"
	"strings"
)

type AgentCommand struct {
	ScriptLine string
	*AgentLogger
	Expansions  *command.Expansions
	KillChannel chan bool
}

var InterruptedCmdError = errors.New("Command interrupted")

//Run will execute the command in workingDir, by applying the expansions to
//the script and then invoking it with sh -c, and logging all of the command's
//stdout/stderr using the AgentLogger.
//It will block until the command either finishes, or is aborted prematurely
//via the kill channel.
func (self *AgentCommand) Run(workingDir string) error {
	self.LogTask(
		slogger.INFO,
		"Running script task for command \n%v in directory %v",
		self.ScriptLine,
		workingDir)

	logWriterInfo := &mci.LoggingWriter{self.AgentLogger.TaskLogger, slogger.INFO}
	logWriterErr := &mci.LoggingWriter{self.AgentLogger.TaskLogger, slogger.ERROR}

	ignoreErrors := false
	if strings.HasPrefix(self.ScriptLine, "-") {
		self.ScriptLine = self.ScriptLine[1:]
		ignoreErrors = true
	}

	outBufferWriter := util.NewLineBufferingWriter(logWriterInfo)
	errorBufferWriter := util.NewLineBufferingWriter(logWriterErr)
	defer outBufferWriter.Flush()
	defer errorBufferWriter.Flush()

	cmd := &command.LocalCommand{
		CmdString:        self.ScriptLine,
		WorkingDirectory: workingDir,
		Stdout:           outBufferWriter,
		Stderr:           errorBufferWriter,
		Environment:      os.Environ(),
	}
	err := cmd.PrepToRun(self.Expansions)
	if err != nil {
		self.LogTask(slogger.ERROR, "Failed to prepare command: %v", err)
	}

	self.LogTask(slogger.INFO, "Running command (expanded): %v", cmd.CmdString)

	doneStatus := make(chan error)
	go func() {
		err := cmd.Run()
		doneStatus <- err
	}()

	select {
	case err = <-doneStatus:
		if ignoreErrors {
			return nil
		} else {
			return err
		}
	case _ = <-self.KillChannel:
		//try and kill the process
		self.LogExecution(slogger.INFO, "Got kill signal, stopping process: %v", cmd.Cmd.Process.Pid)
		if err := cmd.Stop(); err != nil {
			self.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
		}
		return InterruptedCmdError
	}

	return err
}
