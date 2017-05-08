package agent

import (
	"fmt"
	"os"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

const filenameTimestamp = "2006-01-02_15_04_05"

// SetupLogging configures the agent's local logging to a file.
func SetupLogging(prefix string) error {
	logFile := fmt.Sprintf("%s_%s_pid_%d.log",
		prefix, time.Now().Format(filenameTimestamp), os.Getpid())

	sender, err := send.MakeFileLogger(logFile)
	if err != nil {
		return errors.Wrapf(err, "problem constructing  log writer for file %s", logFile)
	}

	return errors.Wrapf(grip.SetSender(sender),
		"problem setting logger to write to %s", logFile)
}
