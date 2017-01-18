package monitor

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/tychoish/grip"
)

type Notifier struct {
	// functions which will be called to create any notifications that need
	// to be sent
	notificationBuilders []notificationBuilder
}

// create and send any notifications that need to be sent
func (self *Notifier) Notify(settings *evergreen.Settings) []error {
	grip.Info("Building and sending necessary notifications...")

	// used to store any errors that occur
	var errors []error

	for _, f := range self.notificationBuilders {

		// get the necessary notifications
		notifications, err := f(settings)

		// continue on error so that one wonky function doesn't stop the others
		// from running
		if err != nil {
			errors = append(errors, fmt.Errorf("error building"+
				" notifications to be sent: %v", err))
			continue
		}

		// send the actual notifications. continue on error to allow further
		// notifications to be sent
		if errs := sendNotifications(notifications, settings); errs != nil {
			for _, err := range errs {
				errors = append(errors, fmt.Errorf("error sending"+
					" notifications: %v", err))
			}
			continue
		}

	}

	grip.Info("Done building and sending notifications")

	return errors
}

// send all of the specified notifications, and execute the callbacks for any
// that are successfully sent. returns an aggregate list of any errors
// that occur
func sendNotifications(notifications []notification,
	settings *evergreen.Settings) []error {

	grip.Infof("Sending %d notifications...", len(notifications))

	// used to store any errors that occur
	var errors []error

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
			errors = append(errors, fmt.Errorf("error sending notification"+
				" to %v: %v", n.recipient, err))
			continue
		}

		// run the notification's callback, since it has been successfully sent
		if n.callback != nil {
			if err := n.callback(n.host, n.threshold); err != nil {
				errors = append(errors, fmt.Errorf("error running notification"+
					" callback: %v", err))
			}
		}

	}

	return errors
}
