package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type Notifier struct {
	// functions which will be called to create any notifications that need
	// to be sent
	NotificationBuilders []NotificationBuilder
}

// create and send any notifications that need to be sent
func (self *Notifier) Notify(settings *evergreen.Settings) error {
	// used to store any errors that occur
	catcher := grip.NewBasicCatcher()
	startAt := time.Now()

	for _, f := range self.NotificationBuilders {

		// get the necessary notifications
		notifications, err := f(settings)

		// continue on error so that one wonky function doesn't stop the others
		// from running
		if err != nil {
			catcher.Add(errors.Wrap(err,
				"error building notifications to be sent"))
			continue
		}

		// send the actual notifications. continue on error to allow further
		// notifications to be sent
		catcher.Extend(sendNotifications(notifications, settings))
	}

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"message":       "completed monitor notifications",
		"num_builders":  len(self.NotificationBuilders),
		"num_errors":    catcher.Len(),
		"duration_secs": time.Since(startAt).Seconds(),
	})

	return catcher.Resolve()
}

// send all of the specified notifications, and execute the callbacks for any
// that are successfully sent. returns an aggregate list of any errors
// that occur
func sendNotifications(notifications []Notification, settings *evergreen.Settings) []error {
	// used to store any errors that occur
	var errs []error

	mailer, err := evergreen.GetEnvironment().GetSender(evergreen.SenderEmail)
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	for _, n := range notifications {
		mailer.Send(message.NewEmailMessage(level.Notice, message.Email{
			From:       settings.Notify.SMTP.From,
			Recipients: []string{n.recipient},
			Subject:    n.subject,
			Body:       n.message,
		}))

		// run the notification's callback, since it has been successfully sent
		if n.callback != nil {
			if err := n.callback(n.host, n.threshold); err != nil {
				errs = append(errs, errors.Wrap(err,
					"error running notification callback"))
			}
		}

	}

	return errs
}
