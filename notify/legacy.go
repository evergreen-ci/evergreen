package notify

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	RunnerName = "notify.legacy"

	// Thus on each run of the notifier, we check if the last notification time
	// (LNT) is within this window. If it is, then we use the retrieved LNT.
	// If not, we use the current time
	LNRWindow = 60 * time.Minute

	// MCI ops notification prefaces
	ProvisionFailurePreface = "[PROVISION-FAILURE]"
	ProvisionTimeoutPreface = "[PROVISION-TIMEOUT]"
	ProvisionLatePreface    = "[PROVISION-LATE]"
	TeardownFailurePreface  = "[TEARDOWN-FAILURE]"

	// repotracker notification prefaces
	RepotrackerFailurePreface = "[REPOTRACKER-FAILURE %v] on %v"

	// branch name to use if the project reference is not found
	UnknownProjectBranch = ""

	DefaultNotificationsConfig = "/etc/mci-notifications.yml"

	// number of times to try sending a notification
	numSmtpRetries = 3
	smtpSleepTime  = 1 * time.Second
)

func ConstructMailer(notifyConfig evergreen.NotifyConfig) Mailer {
	if notifyConfig.SMTP != nil {
		return SmtpMailer{
			notifyConfig.SMTP.From,
			notifyConfig.SMTP.Server,
			notifyConfig.SMTP.Port,
			notifyConfig.SMTP.UseSSL,
			notifyConfig.SMTP.Username,
			notifyConfig.SMTP.Password,
		}
	}

	// shouldn't panic in production code, but this is temporary,
	// as this code is deprecated and shouldn't happen anyway
	panic("no configured notify smtp settings")
}

// NotifyAdmins is a helper method to send a notification to the MCI admin team
func NotifyAdmins(subject, msg string, settings *evergreen.Settings) error {
	if settings.Notify.SMTP != nil {
		return TrySendNotification(settings.Notify.SMTP.AdminEmail, subject, msg, ConstructMailer(settings.Notify))
	}
	err := errors.New("Cannot notify admins: admin_email not set")

	grip.Error(message.WrapError(err, message.Fields{
		"runner":  RunnerName,
		"message": msg,
		"subject": subject,
	}))

	return err
}

// Helper function to send notifications
func TrySendNotification(recipients []string, subject, body string, mailer Mailer) (err error) {
	// grip.Debugf("address: %s subject: %s body: %s", recipients, subject, body)
	// return nil
	_, err = util.Retry(func() (bool, error) {
		err = mailer.SendMail(recipients, subject, body)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"message": "sending notification",
			}))
			return true, err
		}
		return false, nil
	}, numSmtpRetries, smtpSleepTime)
	return errors.WithStack(err)
}

func TrySendNotificationToUser(userId string, subject, body string, mailer Mailer) error {
	dbUser, err := user.FindOne(user.ById(userId))
	if err != nil {
		return errors.Wrapf(err, "Error finding user %v", userId)
	} else if dbUser == nil {
		return errors.Errorf("User %v not found", userId)
	} else {
		return errors.WithStack(TrySendNotification([]string{dbUser.Email()}, subject, body, mailer))
	}
}
