package agent

import (
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// DumpStackOnSIGQUIT listens for a SIGQUIT signal and writes stack dump to the
// given io.Writer when one is received. Blocks, so spawn it as a goroutine.
func DumpStackOnSIGQUIT(curAgent **Agent) {
	in := make(chan os.Signal)
	signal.Notify(in, syscall.SIGQUIT)
	for _ = range in {
		agt := *curAgent
		stackBytes := trace()
		task, command := taskAndCommand(agt)

		// we dump to files and logs without blocking, in case our logging is deadlocked or broken
		go dumpToDisk(task, command, stackBytes)
		go dumpToLogs(task, command, stackBytes, agt)
	}
}

func trace() []byte {
	var buf [32 * 1024]byte
	n := runtime.Stack(buf[:], true)
	return buf[:n]
}

func taskAndCommand(agt *Agent) (string, string) {
	task := "no running task"
	command := "no running command"
	if agt != nil {
		if agt.taskConfig != nil {
			task = agt.taskConfig.Task.Id
		}
		if cmd := agt.GetCurrentCommand(); cmd.Command != "" {
			command = cmd.Command
		}
	}
	return task, command

}

func dumpToLogs(task, command string, stack []byte, agt *Agent) {
	if agt != nil && agt.logger != nil {
		logWriter := evergreen.NewInfoLoggingWriter(agt.logger.System)
		dumpDebugInfo(task, command, stack, logWriter)
	}
}

func dumpToDisk(task, command string, stack []byte) {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	dumpFile, err := ioutil.TempFile(wd, newDumpFilename())
	if err != nil {
		return // fail silently -- things are very wrong
	}
	defer dumpFile.Close()
	dumpDebugInfo(task, command, stack, dumpFile)
}

func newDumpFilename() string {
	// e.g. "evergreen_agent_46672_dump_2015-09-08T23:26:49-04:00..."
	return fmt.Sprintf("evergreen_agent_%v_dump_%v_", os.Getpid(), time.Now().Format(time.RFC3339))
}

func dumpDebugInfo(task, command string, stack []byte, w io.Writer) {
	out := bytes.Buffer{}
	out.WriteString(fmt.Sprintf("Agent stack dump taken on %v.\n\n", time.Now().Format(time.UnixDate)))
	out.WriteString(fmt.Sprintf("Running command '%v' for task '%v'.\n\n", command, task))
	out.Write(stack)
	w.Write(out.Bytes())
}
