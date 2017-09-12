package agent

import (
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

var idSource chan int

func init() {
	idSource = make(chan int, 100)

	go func() {
		id := 0
		for {
			idSource <- id
			id++
		}
	}()
}

func getInc() int { return <-idSource }

// getSender configures the agent's local logging to a file.
func getSender(prefix, taskId string) (send.Sender, error) {
	var (
		err     error
		sender  send.Sender
		senders []send.Sender
	)

	level := grip.GetSender().Level()

	if splunk := send.GetSplunkConnectionInfo(); splunk.Populated() {
		sender, err = send.NewSplunkLogger(taskId, splunk, level)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the splunk logger")
		}

		senders = append(senders, send.NewBufferedSender(sender, 15*time.Second, 100))
	}

	if endpoint := os.Getenv("GRIP_SUMO_ENDPOINT"); endpoint != "" {
		sender, err = send.NewSumo(taskId, endpoint)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the sumo logic sender")
		}
		senders = append(senders, send.NewBufferedSender(sender, 15*time.Second, 100))
	}

	if prefix == "" {
		// pass
	} else if prefix == evergreen.LocalLoggingOverride || prefix == "--" {
		sender, err = send.NewNativeLogger("evergreen-agent", level)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a native console logger")
		}

		senders = append(senders, sender)
	} else {
		sender, err = send.NewFileLogger("evergreen-agent",
			fmt.Sprintf("%s-%d-%d.log", prefix, os.Getpid(), getInc()), level)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a file logger")
		}

		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), nil
}
