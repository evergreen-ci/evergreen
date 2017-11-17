package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/grip/level"
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

// GetSender configures the agent's local logging to a file.
func GetSender(ctx context.Context, prefix, taskId string) (send.Sender, error) {
	var (
		err     error
		sender  send.Sender
		senders []send.Sender
	)

	if splunk := send.GetSplunkConnectionInfo(); splunk.Populated() {
		sender, err = send.NewSplunkLogger("evergreen.agent", splunk, send.LevelInfo{Default: level.Info, Threshold: level.Alert})
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the splunk logger")
		}

		sender, err = logger.NewQueueBackedSender(ctx, sender, 2, 10)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the splunk buffer")
		}

		senders = append(senders, sender)
	}

	if endpoint := os.Getenv("GRIP_SUMO_ENDPOINT"); endpoint != "" {
		sender, err = send.NewSumo(taskId, endpoint)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the sumo logic sender")
		}
		if err = sender.SetLevel(send.LevelInfo{Default: level.Info, Threshold: level.Alert}); err != nil {
			return nil, errors.Wrap(err, "problem setting level for alert remote object")
		}

		sender, err = logger.NewQueueBackedSender(ctx, sender, 2, 10)
		if err != nil {
			return nil, errors.Wrap(err, "problem creating the splunk buffer")
		}

		senders = append(senders, sender)
	}

	if prefix == "" {
		// pass
	} else if prefix == evergreen.LocalLoggingOverride || prefix == "--" {
		sender, err = send.NewNativeLogger("evergreen.agent", send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a native console logger")
		}

		senders = append(senders, sender)
	} else {
		sender, err = send.NewFileLogger("evergreen.agent",
			fmt.Sprintf("%s-%d-%d.log", prefix, os.Getpid(), getInc()), send.LevelInfo{Default: level.Info, Threshold: level.Debug})
		if err != nil {
			return nil, errors.Wrap(err, "problem creating a file logger")
		}

		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), nil
}
