package agent

import (
	"errors"
	"os"
	"strings"

	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/util"
)

// AgentCommand encapsulates a running local command and streams logs
// back to the API server.
type AgentCommand struct {
	*comm.StreamLogger
	ScriptLine string
	Expansions *command.Expansions
	KillChan   chan bool
}

// InterruptedCmdError is returned by commands that were stopped
// before they could complete.
var InterruptedCmdError = errors.New("Command interrupted")

// Run will execute the command in workingDir, by applying the expansions to
// the script and then invoking it with sh -c, and logging all of the command's
// stdout/stderr using the Logger.
// It will block until the command either finishes, or is aborted prematurely
// via the kill channel.
func (ac *AgentCommand) Run(workingDir string) error {
	ac.LogTask(slogger.INFO, "Running script task for command \n%v in directory %v", ac.ScriptLine, workingDir)

	logWriterInfo := &evergreen.LoggingWriter{ac.Task, slogger.INFO}
	logWriterErr := &evergreen.LoggingWriter{ac.Task, slogger.ERROR}

	ignoreErrors := false
	if strings.HasPrefix(ac.ScriptLine, "-") {
		ac.ScriptLine = ac.ScriptLine[1:]
		ignoreErrors = true
	}

	outBufferWriter := util.NewLineBufferingWriter(logWriterInfo)
	errorBufferWriter := util.NewLineBufferingWriter(logWriterErr)
	defer outBufferWriter.Flush()
	defer errorBufferWriter.Flush()

	cmd := &command.LocalCommand{
		CmdString:        ac.ScriptLine,
		WorkingDirectory: workingDir,
		Stdout:           outBufferWriter,
		Stderr:           errorBufferWriter,
		Environment:      os.Environ(),
	}
	err := cmd.PrepToRun(ac.Expansions)
	if err != nil {
		ac.LogTask(slogger.ERROR, "Failed to prepare command: %v", err)
	}

	ac.LogTask(slogger.INFO, "Running command (expanded): %v", cmd.CmdString)

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
	case _ = <-ac.KillChan:
		// try and kill the process
		ac.LogExecution(slogger.INFO, "Got kill signal, stopping process: %v", cmd.GetPid())
		if err := cmd.Stop(); err != nil {
			ac.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
		}
		return InterruptedCmdError
	}

	return err
}
