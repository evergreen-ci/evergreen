package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/notify"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
)

type Notifier struct {
	// functions which will be called to create any notifications that need
	// to be sent
	notificationBuilders []notificationBuilder
}

// create and send any notifications that need to be sent
func (self *Notifier) Notify(mciSettings *mci.MCISettings) []error {

	mci.Logger.Logf(slogger.INFO, "Building and sending necessary"+
		" notifications...")

	// used to store any errors that occur
	var errors []error

	for _, f := range self.notificationBuilders {

		// get the necessary notifications
		notifications, err := f(mciSettings)

		// continue on error so that one wonky function doesn't stop the others
		// from running
		if err != nil {
			errors = append(errors, fmt.Errorf("error building"+
				" notifications to be sent: %v", err))
			continue
		}

		// send the actual notifications. continue on error to allow further
		// notifications to be sent
		if errs := sendNotifications(notifications, mciSettings); errs != nil {
			for _, err := range errs {
				errors = append(errors, fmt.Errorf("error sending"+
					" notifications: %v", err))
			}
			continue
		}

	}

	mci.Logger.Logf(slogger.INFO, "Done building and sending notifications")

	return errors
}

// send all of the specified notifications, and execute the callbacks for any
// that are successfully sent. returns an aggregate list of any errors
// that occur
func sendNotifications(notifications []notification,
	mciSettings *mci.MCISettings) []error {

	mci.Logger.Logf(slogger.INFO, "Sending %v notifications...",
		len(notifications))

	// used to store any errors that occur
	var errors []error

	// ask for the mailer we'll use
	mailer := notify.ConstructMailer(mciSettings.Notify)

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
