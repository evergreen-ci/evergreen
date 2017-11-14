package notify

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	// number of times to try sending a notification
	NumSmtpRetries = 3

	// period to sleep between tries
	SmtpSleepTime = 1 * time.Second

	// smtp port to connect to
	SmtpPort = 25

	// smtp relay host to connect to
	SmtpServer = "localhost"

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
)

var (
	// notifications can be either build-level or task-level
	// we might subsequently want to submit js test notifications
	// within each task
	buildType = "build"
	taskType  = "task"

	// notification key types
	buildFailureKey          = "build_failure"
	buildSuccessKey          = "build_success"
	buildSuccessToFailureKey = "build_success_to_failure"
	buildCompletionKey       = "build_completion"
	taskFailureKey           = "task_failure"
	taskSuccessKey           = "task_success"
	taskSuccessToFailureKey  = "task_success_to_failure"
	taskCompletionKey        = "task_completion"
	taskFailureKeys          = []string{taskFailureKey, taskSuccessToFailureKey}
	buildFailureKeys         = []string{buildFailureKey, buildSuccessToFailureKey}

	// notification subjects
	failureSubject    = "failed"
	completionSubject = "completed"
	successSubject    = "succeeded"
	transitionSubject = "transitioned to failure"

	// task/build status notification prefaces
	mciSuccessPreface    = "[MCI-SUCCESS %v]"
	mciFailurePreface    = "[MCI-FAILURE %v]"
	mciCompletionPreface = "[MCI-COMPLETION %v]"

	// patch notification prefaces
	patchSuccessPreface    = "[PATCH-SUCCESS %v]"
	patchFailurePreface    = "[PATCH-FAILURE %v]"
	patchCompletionPreface = "[PATCH-COMPLETION %v]"

	buildNotificationHandler = BuildNotificationHandler{buildType}
	taskNotificationHandler  = TaskNotificationHandler{taskType}

	Handlers = map[string]NotificationHandler{
		buildFailureKey:          &BuildFailureHandler{buildNotificationHandler, buildFailureKey},
		buildSuccessKey:          &BuildSuccessHandler{buildNotificationHandler, buildSuccessKey},
		buildCompletionKey:       &BuildCompletionHandler{buildNotificationHandler, buildCompletionKey},
		buildSuccessToFailureKey: &BuildSuccessToFailureHandler{buildNotificationHandler, buildSuccessToFailureKey},
		taskFailureKey:           &TaskFailureHandler{taskNotificationHandler, taskFailureKey},
		taskSuccessKey:           &TaskSuccessHandler{taskNotificationHandler, taskSuccessKey},
		taskCompletionKey:        &TaskCompletionHandler{taskNotificationHandler, taskCompletionKey},
		taskSuccessToFailureKey:  &TaskSuccessToFailureHandler{taskNotificationHandler, taskSuccessToFailureKey},
	}
)

var (
	// These help us to limit the tasks/builds we pull
	// from the database on each run of the notifier
	lastProjectNotificationTime = make(map[string]time.Time)
	newProjectNotificationTime  = make(map[string]time.Time)

	// Simple map for optimization
	cachedProjectRecords = make(map[string]interface{})

	// Simple map for caching project refs
	cachedProjectRef = make(map[string]*model.ProjectRef)
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
	} else {
		return SmtpMailer{
			Server: SmtpServer,
			Port:   SmtpPort,
			UseSSL: false,
		}
	}
}

// This function is responsible for running the notifications pipeline
//
// ParseNotifications
//         ↓↓
// ValidateNotifications
//         ↓↓
// ProcessNotifications
//         ↓↓
// SendNotifications
//         ↓↓
// UpdateNotificationTimes
//
func Run(settings *evergreen.Settings) error {
	// get the notifications
	mciNotification, err := ParseNotifications(settings.ConfigDir)
	if err != nil {
		grip.Errorf("parsing notifications: %+v", err)
		return err
	}

	// validate the notifications
	err = ValidateNotifications(mciNotification)
	if err != nil {
		grip.Errorf("validating notifications: %+v", err)
		return err
	}

	templateGlobals := map[string]interface{}{
		"UIRoot": settings.Ui.Url,
	}

	ae, err := createEnvironment(settings, templateGlobals)
	if err != nil {
		return err
	}

	// process the notifications
	emails, err := ProcessNotifications(ae, mciNotification, true)
	if err != nil {
		grip.Errorf("processing notifications: %+v", err)
		return err
	}

	// Remnants are outstanding build/task notifications that
	// we couldn't process on a prior run of the notifier
	// Currently, this can only happen if an administrator
	// bumps up the priority of a build/task

	// send the notifications

	err = SendNotifications(settings, mciNotification, emails,
		ConstructMailer(settings.Notify))
	if err != nil {
		grip.Errorln("Error sending notifications:", err)
		return err
	}

	// update notification times
	err = UpdateNotificationTimes()
	if err != nil {
		grip.Errorln("Error updating notification times:", err)
		return err
	}
	return nil
}

// This function is responsible for reading the notifications file
func ParseNotifications(configName string) (*MCINotification, error) {
	grip.Info("Parsing notifications...")

	evgHome := evergreen.FindEvergreenHome()
	configs := []string{
		filepath.Join(evgHome, configName, evergreen.NotificationsFile),
		filepath.Join(evgHome, evergreen.NotificationsFile),
		DefaultNotificationsConfig,
	}

	var notificationsFile string
	for _, fn := range configs {
		if _, err := os.Stat(fn); os.IsNotExist(err) {
			continue
		}

		notificationsFile = fn
	}

	data, err := ioutil.ReadFile(notificationsFile)
	if err != nil {
		return nil, err
	}

	// unmarshal file contents into MCINotification struct
	mciNotification := &MCINotification{}

	err = yaml.Unmarshal(data, mciNotification)
	if err != nil {
		return nil, errors.Wrapf(err, "Parse error unmarshalling notifications %v", notificationsFile)
	}
	return mciNotification, nil
}

// This function is responsible for validating the notifications file
func ValidateNotifications(mciNotification *MCINotification) error {
	grip.Info("Validating notifications...")
	allNotifications := []string{}

	projectNameToBuildVariants, err := findProjectBuildVariants()
	if err != nil {
		return errors.Wrap(err, "Error loading project build variants")
	}

	// Validate default notification recipients
	for _, notification := range mciNotification.Notifications {
		if notification.Project == "" {
			return errors.Errorf("Must specify a project for each notification - see %v", notification.Name)
		}

		buildVariants, ok := projectNameToBuildVariants[notification.Project]
		if !ok {
			return errors.Errorf("Notifications validation failed: "+
				"project `%v` not found", notification.Project)
		}

		// ensure all supplied build variants are valid
		for _, buildVariant := range notification.SkipVariants {
			if !util.StringSliceContains(buildVariants, buildVariant) {
				return errors.Errorf("Nonexistent buildvariant - ”%v” - specified for ”%v” notification", buildVariant, notification.Name)
			}
		}

		allNotifications = append(allNotifications, notification.Name)
	}

	// Validate team name and addresses
	for _, team := range mciNotification.Teams {
		if team.Name == "" {
			return errors.Errorf("Each notification team must have a name")
		}

		for _, subscription := range team.Subscriptions {
			for _, notification := range subscription.NotifyOn {
				if !util.StringSliceContains(allNotifications, notification) {
					return errors.Errorf("Team ”%v” contains a non-existent subscription - %v", team.Name, notification)
				}
			}
			for _, buildVariant := range subscription.SkipVariants {
				buildVariants, ok := projectNameToBuildVariants[subscription.Project]
				if !ok {
					return errors.Errorf("Teams validation failed: project `%v` not found", subscription.Project)
				}

				if !util.StringSliceContains(buildVariants, buildVariant) {
					return errors.Errorf("Nonexistent buildvariant - ”%v” - specified for team ”%v” ", buildVariant, team.Name)
				}
			}
		}
	}

	// Validate patch notifications
	for _, subscription := range mciNotification.PatchNotifications {
		for _, notification := range subscription.NotifyOn {
			if !util.StringSliceContains(allNotifications, notification) {
				return errors.Errorf("Nonexistent patch notification - ”%v” - specified", notification)
			}
		}
	}

	// validate the patch notification buildvariatns
	for _, subscription := range mciNotification.PatchNotifications {
		buildVariants, ok := projectNameToBuildVariants[subscription.Project]
		if !ok {
			return errors.Errorf("Patch notification build variants validation failed: "+
				"project `%v` not found", subscription.Project)
		}

		for _, buildVariant := range subscription.SkipVariants {
			if !util.StringSliceContains(buildVariants, buildVariant) {
				return errors.Errorf("Nonexistent buildvariant - ”%v” - specified for patch notifications", buildVariant)
			}
		}
	}

	// all good!
	return nil
}

// This function is responsible for all notifications processing
func ProcessNotifications(ae *web.App, mciNotification *MCINotification, updateTimes bool) (map[NotificationKey][]Email, error) {
	// create MCI notifications
	allNotificationsSlice := notificationsToStruct(mciNotification)

	// get the last notification time for all projects
	if updateTimes {
		err := getLastProjectNotificationTime(allNotificationsSlice)
		if err != nil {
			return nil, err
		}
	}

	grip.Info("Processing notifications...")

	emails := make(map[NotificationKey][]Email)
	for _, key := range allNotificationsSlice {
		emailsForKey, err := Handlers[key.NotificationName].GetNotifications(ae, &key)
		if err != nil {
			grip.Infof("Error processing %s on %s: %+v", key.NotificationName, key.Project, err)
			continue
		}
		emails[key] = emailsForKey
	}

	return emails, nil
}

// This function is responsible for managing the sending triggered email notifications
func SendNotifications(settings *evergreen.Settings, mciNotification *MCINotification,
	emails map[NotificationKey][]Email, mailer Mailer) (err error) {

	grip.Info("Sending notifications...")

	// parse all notifications, sending it to relevant recipients
	for _, notification := range mciNotification.Notifications {
		key := NotificationKey{
			Project:               notification.Project,
			NotificationName:      notification.Name,
			NotificationType:      getType(notification.Name),
			NotificationRequester: evergreen.RepotrackerVersionRequester,
		}

		for _, recipient := range notification.Recipients {
			// send all triggered notifications
			for _, email := range emails[key] {

				// determine if this notification should be skipped - based on the buildvariant
				if email.ShouldSkip(notification.SkipVariants) {
					continue
				}

				// send to individual subscriber, or the admin team if it's not their fault
				recipients := []string{}
				if settings.Notify.SMTP != nil {
					recipients = settings.Notify.SMTP.AdminEmail
				}
				if !email.IsLikelySystemFailure() {
					recipients = email.GetRecipients(recipient)
				}
				err = TrySendNotification(recipients, email.GetSubject(), email.GetBody(), mailer)
				if err != nil {
					grip.Errorf("Unable to send individual notification %#v: %+v", key, err)
					continue
				}
			}
		}
	}

	// Send team subscribed notifications
	for _, team := range mciNotification.Teams {
		for _, subscription := range team.Subscriptions {
			for _, name := range subscription.NotifyOn {
				key := NotificationKey{
					Project:               subscription.Project,
					NotificationName:      name,
					NotificationType:      getType(name),
					NotificationRequester: evergreen.RepotrackerVersionRequester,
				}

				// send all triggered notifications for this key
				for _, email := range emails[key] {
					// determine if this notification should be skipped - based on the buildvariant
					if email.ShouldSkip(subscription.SkipVariants) {
						continue
					}

					teamEmail := fmt.Sprintf("%v <%v>", team.Name, team.Address)
					err = TrySendNotification([]string{teamEmail}, email.GetSubject(), email.GetBody(), mailer)
					if err != nil {
						grip.Errorf("Unable to send notification %#v: %v", key, err)
						continue
					}
				}
			}
		}
	}

	// send patch notifications
	/* XXX temporarily disable patch notifications
	for _, subscription := range mciNotification.PatchNotifications {
		for _, notification := range subscription.NotifyOn {
			key := NotificationKey{
				Project:               subscription.Project,
				NotificationName:      notification,
				NotificationType:      getType(notification),
				NotificationRequester: evergreen.PatchVersionRequester,
			}

			for _, email := range emails[key] {
				// determine if this notification should be skipped -
				// based on the buildvariant
				if email.ShouldSkip(subscription.SkipVariants) {
					continue
				}

				// send to the appropriate patch requester
				for _, changeInfo := range email.GetChangeInfo() {
					// send notification to each member of the blamelist
					patchRequester := fmt.Sprintf("%v <%v>", changeInfo.Author,
						changeInfo.Email)
					err = TrySendNotification([]string{patchRequester},
						email.GetSubject(), email.GetBody(), mailer)
					if err != nil {
						grip.Errorf("Unable to send notification %#v: %+v", key, err)
						continue
					}
				}
			}
		}
	}*/

	return nil
}

// This stores the last time threshold after which
// we search for possible new notification events
func UpdateNotificationTimes() (err error) {
	grip.Info("Updating notification times...")
	for project, time := range newProjectNotificationTime {
		grip.Infof("Updating %s notification time...", project)
		err = model.SetLastNotificationsEventTime(project, time)
		if err != nil {
			return err
		}
	}
	return nil
}

//***********************************\/
//   Notification Helper Functions   \/
//***********************************\/

// Construct a map of project names to build variants for that project
func findProjectBuildVariants() (map[string][]string, error) {
	projectNameToBuildVariants := make(map[string][]string)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, err
	}

	for _, projectRef := range allProjects {
		if !projectRef.Enabled {
			continue
		}
		var buildVariants []string
		var proj *model.Project
		var err error
		if projectRef.LocalConfig != "" {
			proj, err = model.FindProject("", &projectRef)
			if err != nil {
				return nil, errors.Wrap(err, "unable to find project file")
			}
		} else {
			lastGood, err := version.FindOne(version.ByLastKnownGoodConfig(projectRef.Identifier))
			if err != nil {
				return nil, errors.Wrap(err, "unable to find last valid config")
			}
			if lastGood == nil { // brand new project + no valid config yet, just return an empty map
				return projectNameToBuildVariants, nil
			}

			proj = &model.Project{}
			err = model.LoadProjectInto([]byte(lastGood.Config), projectRef.Identifier, proj)
			if err != nil {
				return nil, errors.Wrapf(err, "error loading project '%v' from version",
					projectRef.Identifier)
			}
		}

		for _, buildVariant := range proj.BuildVariants {
			buildVariants = append(buildVariants, buildVariant.Name)
		}

		projectNameToBuildVariants[projectRef.Identifier] = buildVariants
	}

	return projectNameToBuildVariants, nil
}

// construct the change information
// struct from a given version struct
func constructChangeInfo(v *version.Version, notification *NotificationKey) (changeInfo *ChangeInfo) {
	changeInfo = &ChangeInfo{}
	switch notification.NotificationRequester {
	case evergreen.RepotrackerVersionRequester:
		changeInfo.Project = v.Identifier
		changeInfo.Author = v.Author
		changeInfo.Message = v.Message
		changeInfo.Revision = v.Revision
		changeInfo.Email = v.AuthorEmail

	case evergreen.PatchVersionRequester:
		// get the author and description from the patch request
		patch, err := patch.FindOne(patch.ByVersion(v.Id))
		if err != nil {
			grip.Errorf("Error finding patch for version %s: %+v", v.Id, err)
			return
		}

		if patch == nil {
			grip.Errorln(notification, "notification was unable to locate patch with version:", v.Id)
			return
		}
		// get the display name and email for this user
		dbUser, err := user.FindOne(user.ById(patch.Author))
		if err != nil {
			grip.Errorf("Error finding user %s: %+v", patch.Author, err)
			changeInfo.Author = patch.Author
			changeInfo.Email = patch.Author
		} else if dbUser == nil {
			grip.Errorf("User %s not found", patch.Author)
			changeInfo.Author = patch.Author
			changeInfo.Email = patch.Author
		} else {
			changeInfo.Email = dbUser.Email()
			changeInfo.Author = dbUser.DisplayName()
		}

		changeInfo.Project = patch.Project
		changeInfo.Message = patch.Description
		changeInfo.Revision = patch.Id.Hex()
	}
	return
}

// get the display name for a build variant's
// task given the build variant name
func getDisplayName(buildVariant string) (displayName string) {
	build, err := build.FindOne(build.ByVariant(buildVariant))
	if err != nil || build == nil {
		grip.Error(errors.Wrap(err, "Error fetching buildvariant name"))
		displayName = buildVariant
	} else {
		displayName = build.DisplayName
	}
	return
}

// get the failed task(s) for a given build
func getFailedTasks(current *build.Build, notificationName string) (failedTasks []build.TaskCache) {
	if util.StringSliceContains(buildFailureKeys, notificationName) {
		for _, t := range current.Tasks {
			if t.Status == evergreen.TaskFailed {
				failedTasks = append(failedTasks, t)
			}
		}
	}
	return
}

// get the specific failed test(s) for this task
func getFailedTests(current *task.Task, notificationName string) (failedTests []task.TestResult) {
	if util.StringSliceContains(taskFailureKeys, notificationName) {
		for _, test := range current.LocalTestResults {
			if test.Status == "fail" {
				// get the base name for windows/non-windows paths
				test.TestFile = path.Base(strings.Replace(test.TestFile, "\\", "/", -1))
				failedTests = append(failedTests, test)
			}
		}
	}

	return
}

// gets the project ref project name corresponding to this identifier
func getProjectRef(identifier string) (projectRef *model.ProjectRef,
	err error) {
	if cachedProjectRef[identifier] == nil {
		projectRef, err = model.FindOneProjectRef(identifier)
		if err != nil {
			return
		}
		cachedProjectRecords[identifier] = projectRef
		return projectRef, nil
	}
	return cachedProjectRef[identifier], nil
}

// This gets the time threshold - events before which
// we searched for possible notification events
func getLastProjectNotificationTime(keys []NotificationKey) error {
	for _, key := range keys {
		lastNotificationTime, err := model.LastNotificationsEventTime(key.Project)
		if err != nil {
			return errors.WithStack(err)
		}
		if lastNotificationTime.Before(time.Now().Add(-LNRWindow)) {
			lastNotificationTime = time.Now()
		}
		lastProjectNotificationTime[key.Project] = lastNotificationTime
		newProjectNotificationTime[key.Project] = time.Now()
	}
	return nil
}

// This is used to pull recently finished builds
func getRecentlyFinishedBuilds(notificationKey *NotificationKey) (builds []build.Build, err error) {
	if cachedProjectRecords[notificationKey.String()] == nil {
		builds, err = build.Find(build.ByFinishedAfter(lastProjectNotificationTime[notificationKey.Project], notificationKey.Project, notificationKey.NotificationRequester))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cachedProjectRecords[notificationKey.String()] = builds
		return builds, errors.WithStack(err)
	}
	return cachedProjectRecords[notificationKey.String()].([]build.Build), nil
}

// This is used to pull recently finished tasks
func getRecentlyFinishedTasks(notificationKey *NotificationKey) (tasks []task.Task, err error) {
	if cachedProjectRecords[notificationKey.String()] == nil {

		tasks, err = task.Find(task.ByRecentlyFinished(lastProjectNotificationTime[notificationKey.Project],
			notificationKey.Project, notificationKey.NotificationRequester))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cachedProjectRecords[notificationKey.String()] = tasks
		return tasks, errors.WithStack(err)
	}
	return cachedProjectRecords[notificationKey.String()].([]task.Task), nil
}

// gets the type of notification - we support build/task level notification
func getType(notification string) (nkType string) {
	nkType = taskType
	if strings.Contains(notification, buildType) {
		nkType = buildType
	}
	return
}

func notificationKeySliceContains(slice []NotificationKey, item NotificationKey) bool {
	for idx := range slice {
		if slice[idx] == item {
			return true
		}
	}

	return false
}

// creates/returns slice of 'relevant' NotificationKeys a
// notification is relevant if it has at least one recipient
func notificationsToStruct(mciNotification *MCINotification) (notifyOn []NotificationKey) {
	// Get default notifications
	for _, notification := range mciNotification.Notifications {
		if len(notification.Recipients) != 0 {
			// flag the notification as needed
			key := NotificationKey{
				Project:               notification.Project,
				NotificationName:      notification.Name,
				NotificationType:      getType(notification.Name),
				NotificationRequester: evergreen.RepotrackerVersionRequester,
			}

			// prevent duplicate notifications from being sent
			if !notificationKeySliceContains(notifyOn, key) {
				notifyOn = append(notifyOn, key)
			}
		}
	}

	// Get team notifications
	for _, team := range mciNotification.Teams {
		for _, subscription := range team.Subscriptions {
			for _, name := range subscription.NotifyOn {
				key := NotificationKey{
					Project:               subscription.Project,
					NotificationName:      name,
					NotificationType:      getType(name),
					NotificationRequester: evergreen.RepotrackerVersionRequester,
				}

				// prevent duplicate notifications from being sent
				if !notificationKeySliceContains(notifyOn, key) {
					notifyOn = append(notifyOn, key)
				}
			}
		}
	}

	// Get patch notifications
	for _, subscription := range mciNotification.PatchNotifications {
		for _, notification := range subscription.NotifyOn {
			key := NotificationKey{
				Project:               subscription.Project,
				NotificationName:      notification,
				NotificationType:      getType(notification),
				NotificationRequester: evergreen.PatchVersionRequester,
			}

			// prevent duplicate notifications from being sent
			if !notificationKeySliceContains(notifyOn, key) {
				notifyOn = append(notifyOn, key)
			}
		}
	}
	return
}

// NotifyAdmins is a helper method to send a notification to the MCI admin team
func NotifyAdmins(subject, message string, settings *evergreen.Settings) error {
	if settings.Notify.SMTP != nil {
		return TrySendNotification(settings.Notify.SMTP.AdminEmail,
			subject, message, ConstructMailer(settings.Notify))
	}
	err := errors.New("Cannot notify admins: admin_email not set")
	grip.Error(err)
	return err
}

// String method for notification key
func (nk NotificationKey) String() string {
	return fmt.Sprintf("%v-%v-%v", nk.Project, nk.NotificationType, nk.NotificationRequester)
}

// Helper function to send notifications
func TrySendNotification(recipients []string, subject, body string, mailer Mailer) (err error) {
	// grip.Debugf("address: %s subject: %s body: %s", recipients, subject, body)
	// return nil
	_, err = util.Retry(func() (bool, error) {
		err = mailer.SendMail(recipients, subject, body)
		if err != nil {
			grip.Errorln("Error sending notification:", err)
			return true, err
		}
		return false, nil
	}, NumSmtpRetries, SmtpSleepTime)
	return errors.WithStack(err)
}

// Helper function to send notification to a given user
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

//***************************\/
//   Notification Structs    \/
//***************************\/

// stores supported notifications
type Notification struct {
	Name         string   `yaml:"name"`
	Project      string   `yaml:"project"`
	Recipients   []string `yaml:"recipients"`
	SkipVariants []string `yaml:"skip_variants"`
}

// stores notifications subscription for a team
type Subscription struct {
	Project      string   `yaml:"project"`
	SkipVariants []string `yaml:"skip_variants"`
	NotifyOn     []string `yaml:"notify_on"`
}

// stores 10gen team information
type Team struct {
	Name          string         `yaml:"name"`
	Address       string         `yaml:"address"`
	Subscriptions []Subscription `yaml:"subscriptions"`
}

// store notifications file
type MCINotification struct {
	Notifications      []Notification `yaml:"notifications"`
	Teams              []Team         `yaml:"teams"`
	PatchNotifications []Subscription `yaml:"patch_notifications"`
}

// stores high level notifications key
type NotificationKey struct {
	Project               string
	NotificationName      string
	NotificationType      string
	NotificationRequester string
}

// stores information pertaining to a repository's changes
type ChangeInfo struct {
	Revision string
	Author   string
	Email    string
	Pushtime string
	Project  string
	Message  string
}
