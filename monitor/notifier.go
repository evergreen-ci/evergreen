package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type Notifier struct {
	// functions which will be called to create any notifications that need
	// to be sent
	notificationBuilders []notificationBuilder
}

// create and send any notifications that need to be sent
func (self *Notifier) Notify(settings *evergreen.Settings) error {
	grip.Debug(message.Fields{
		"runner":  RunnerName,
		"message": "building and sending notifications",
	})

	// used to store any errors that occur
	catcher := grip.NewBasicCatcher()
	startAt := time.Now()

	for _, f := range self.notificationBuilders {

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

	grip.Debug(message.Fields{
		"runner":        RunnerName,
		"message":       "completed monitor notifications",
		"num_builders":  len(self.notificationBuilders),
		"num_errors":    catcher.Len(),
		"duration_secs": time.Since(startAt).Seconds(),
	})

	return catcher.Resolve()
}

// send all of the specified notifications, and execute the callbacks for any
// that are successfully sent. returns an aggregate list of any errors
// that occur
func sendNotifications(notifications []notification, settings *evergreen.Settings) []error {

	grip.Debug(message.Fields{
		"runner":            RunnerName,
		"message":           "sending notifications",
		"num_notifications": len(notifications),
	})

	// used to store any errors that occur
	var errs []error

	// ask for the mailer we'll use
	mailer := notify.ConstructMailer(settings.Notify)

	for _, n := range notifications {

		// send the notification
		err := notify.TrySendNotificationToUser(
			n.recipient,
			n.subject,
			n.message,
			mailer,
		)

		// continue on error to allow further notifications to be sent
		if err != nil {
			errs = append(errs, errors.Wrapf(err,
				"error sending notification to %s", n.recipient))
			continue
		}

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
