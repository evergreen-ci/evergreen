package notify

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/model/user"
	"10gen.com/mci/model/version"
	"10gen.com/mci/util"
	"10gen.com/mci/web"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/mail"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	// number of times to try sending a notification
	NumSmtpRetries = 3

	// period to sleep between tries
	SmtpSleepTime = time.Duration(1) * time.Second

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

	// repotracker notification prefaces
	RepotrackerFailurePreface = "[REPOTRACKER-FAILURE %v] on %v"

	// branch name to use if the project reference is not found
	UnknownProjectBranch = ""
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
	allFailureKeys           = []string{taskFailureKey, taskSuccessToFailureKey, buildFailureKey, buildSuccessToFailureKey}

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
	// These are helpful for situations where
	// a current task/build has completed but not the
	// previous task/build it is to be compared with
	// *Not Yet Implemented*
	unprocessedBuilds = []string{}
	unprocessedTasks  = []string{}

	// These help us to limit the tasks/builds we pull
	// from the database on each run of the notifier
	lastProjectNotificationTime = make(map[string]time.Time)
	newProjectNotificationTime  = make(map[string]time.Time)

	// Simple map for optimization
	cachedProjectRecords = make(map[string]interface{})

	// Simple map for caching project refs
	cachedProjectRef = make(map[string]*model.ProjectRef)
)

func ConstructMailer(notifyConfig mci.NotifyConfig) Mailer {
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
func Run(mciSettings *mci.MCISettings) error {
	// get the notifications
	mciNotification, err := ParseNotifications(mciSettings.ConfigDir)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error parsing notifications: %v", err)
		return err
	}

	// validate the notifications
	err = ValidateNotifications(mciSettings.ConfigDir, mciNotification)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error validating notifications: %v", err)
		return err
	}

	templateGlobals := map[string]interface{}{
		"UIRoot": mciSettings.Ui.Url,
	}

	ae, err := createEnvironment(mciSettings, templateGlobals)
	if err != nil {
		return err
	}

	// process the notifications
	emails, err := ProcessNotifications(ae, mciSettings.ConfigDir, mciNotification, true)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error processing notifications: %v", err)
		return err
	}

	// Remnants are outstanding build/task notifications that
	// we couldn't process on a prior run of the notifier
	// Currently, this can only happen if an administrator
	// bumps up the priority of a build/task
	// mci.Logger.Logf(slogger.INFO, "Remnant builds %v", len(unprocessedBuilds))
	// mci.Logger.Logf(slogger.INFO, "Remnant tasks %v", len(unprocessedTasks))

	// send the notifications

	err = SendNotifications(mciSettings, mciNotification, emails,
		ConstructMailer(mciSettings.Notify))
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error sending notifications: %v", err)
		return err
	}

	// update notification times
	err = UpdateNotificationTimes()
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating notification times: %v", err)
		return err
	}
	return nil
}

// This function is responsible for reading the notifications file
func ParseNotifications(configName string) (*MCINotification, error) {
	mci.Logger.Logf(slogger.INFO, "Parsing notifications...")
	configRoot, err := mci.FindMCIConfig(configName)
	if err != nil {
		return nil, err
	}

	notificationsFile := filepath.Join(configRoot, mci.NotificationsFile)
	data, err := ioutil.ReadFile(notificationsFile)
	if err != nil {
		return nil, err
	}

	// unmarshal file contents into MCINotification struct
	mciNotification := &MCINotification{}

	err = yaml.Unmarshal(data, mciNotification)
	if err != nil {
		return nil, fmt.Errorf("Parse error unmarshalling notifications %v: %v", notificationsFile, err)
	}
	return mciNotification, nil
}

// This function is responsible for validating the notifications file
func ValidateNotifications(configName string, mciNotification *MCINotification) error {
	mci.Logger.Logf(slogger.INFO, "Validating notifications...")
	allNotifications := []string{}

	projectNameToBuildVariants, err := findProjectBuildVariants(configName)
	if err != nil {
		return fmt.Errorf("Error loading project build variants: %v", err)
	}

	// Validate default notification recipients
	for _, notification := range mciNotification.Notifications {
		if notification.Project == "" {
			return fmt.Errorf("Must specify a project for each notification - see %v", notification.Name)
		}

		buildVariants, ok := projectNameToBuildVariants[notification.Project]
		if !ok {
			return fmt.Errorf("Notifications validation failed: "+
				"project `%v` not found", notification.Project)
		}

		// ensure all supplied build variants are valid
		for _, buildVariant := range notification.SkipVariants {
			if !util.SliceContains(buildVariants, buildVariant) {
				return fmt.Errorf("Nonexistent buildvariant - ”%v” - specified for ”%v” notification", buildVariant, notification.Name)
			}
		}

		allNotifications = append(allNotifications, notification.Name)
	}

	// Validate team name and addresses
	for _, team := range mciNotification.Teams {
		if team.Name == "" {
			return fmt.Errorf("Each notification team must have a name")
		}

		for _, subscription := range team.Subscriptions {
			for _, notification := range subscription.NotifyOn {
				if !util.SliceContains(allNotifications, notification) {
					return fmt.Errorf("Team ”%v” contains a non-existent subscription - %v", team.Name, notification)
				}
			}
			for _, buildVariant := range subscription.SkipVariants {
				buildVariants, ok := projectNameToBuildVariants[subscription.Project]
				if !ok {
					return fmt.Errorf("Teams validation failed: project `%v` not found", subscription.Project)
				}

				if !util.SliceContains(buildVariants, buildVariant) {
					return fmt.Errorf("Nonexistent buildvariant - ”%v” - specified for team ”%v” ", buildVariant, team.Name)
				}
			}
		}
	}

	// Validate patch notifications
	for _, subscription := range mciNotification.PatchNotifications {
		for _, notification := range subscription.NotifyOn {
			if !util.SliceContains(allNotifications, notification) {
				return fmt.Errorf("Nonexistent patch notification - ”%v” - specified", notification)
			}
		}
	}

	// validate the patch notification buildvariatns
	for _, subscription := range mciNotification.PatchNotifications {
		buildVariants, ok := projectNameToBuildVariants[subscription.Project]
		if !ok {
			return fmt.Errorf("Patch notification build variants validation failed: "+
				"project `%v` not found", subscription.Project)
		}

		for _, buildVariant := range subscription.SkipVariants {
			if !util.SliceContains(buildVariants, buildVariant) {
				return fmt.Errorf("Nonexistent buildvariant - ”%v” - specified for patch notifications", buildVariant)
			}
		}
	}

	// all good!
	return nil
}

// This function is responsible for all notifications processing
func ProcessNotifications(ae *web.App, configName string, mciNotification *MCINotification, updateTimes bool) (map[NotificationKey][]Email, error) {
	// create MCI notifications
	allNotificationsSlice := notificationsToStruct(mciNotification)

	// get the last notification time for all projects
	if updateTimes {
		err := getLastProjectNotificationTime(allNotificationsSlice)
		if err != nil {
			return nil, err
		}
	}

	mci.Logger.Logf(slogger.INFO, "Processing notifications...")

	emails := make(map[NotificationKey][]Email)
	for _, key := range allNotificationsSlice {
		emailsForKey, err := Handlers[key.NotificationName].GetNotifications(ae, configName, &key)
		if err != nil {
			mci.Logger.Logf(slogger.INFO, "Error processing %v on %v: %v", key.NotificationName, key.Project, err)
			continue
		}
		emails[key] = emailsForKey
	}

	return emails, nil
}

// This function is responsible for managing the sending triggered email notifications
func SendNotifications(mciSettings *mci.MCISettings, mciNotification *MCINotification,
	emails map[NotificationKey][]Email, mailer Mailer) (err error) {
	mci.Logger.Logf(slogger.INFO, "Sending notifications...")

	// parse all notifications, sending it to relevant recipients
	for _, notification := range mciNotification.Notifications {
		key := NotificationKey{
			Project:               notification.Project,
			NotificationName:      notification.Name,
			NotificationType:      getType(notification.Name),
			NotificationRequester: mci.RepotrackerVersionRequester,
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
				if mciSettings.Notify.SMTP != nil {
					recipients = mciSettings.Notify.SMTP.AdminEmail
				}
				if !email.IsLikelySystemFailure() {
					recipients = email.GetRecipients(recipient)
				}
				err = TrySendNotification(recipients, email.GetSubject(), email.GetBody(), mailer)
				if err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Unable to send "+
						"individual notification %#v: %v", key, err)
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
					NotificationRequester: mci.RepotrackerVersionRequester,
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
						mci.Logger.Errorf(slogger.ERROR, "Unable to send notification %#v: %v", key, err)
						continue
					}
				}
			}
		}
	}

	// send patch notifications
	for _, subscription := range mciNotification.PatchNotifications {
		for _, notification := range subscription.NotifyOn {
			key := NotificationKey{
				Project:               subscription.Project,
				NotificationName:      notification,
				NotificationType:      getType(notification),
				NotificationRequester: mci.PatchVersionRequester,
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
						mci.Logger.Errorf(slogger.ERROR, "Unable to send "+
							"notification %#v: %v", key, err)
						continue
					}
				}
			}
		}
	}

	return nil
}

// This stores the last time threshold after which
// we search for possible new notification events
func UpdateNotificationTimes() (err error) {
	mci.Logger.Logf(slogger.INFO, "Updating notification times...")
	for project, time := range newProjectNotificationTime {
		mci.Logger.Logf(slogger.INFO, "Updating %v notification time...", project)
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
func findProjectBuildVariants(configName string) (map[string][]string, error) {
	projectNameToBuildVariants := make(map[string][]string)

	allProjects, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, err
	}

	for _, projectRef := range allProjects {
		projectName := projectRef.Identifier
		var buildVariants []string

		var proj *model.Project
		var err error
		if !projectRef.Remote {
			proj, err = model.FindProject("", projectName, configName)
			if err != nil {
				return nil, fmt.Errorf("unable to find project file: %v", err)
			}
		} else {
			lastGood, err := version.FindOne(version.ByLastKnownGoodConfig(projectRef.Identifier))
			if err != nil {
				return nil, fmt.Errorf("unable to find last valid config: %v", err)
			}
			if lastGood == nil { // brand new project + no valid config yet, just return an empty map
				return projectNameToBuildVariants, nil
			}

			proj = &model.Project{}
			err = model.LoadProjectInto([]byte(lastGood.Config), proj)
			if err != nil {
				return nil, fmt.Errorf("Error loading project from version: %v", err)
			}
		}

		for _, buildVariant := range proj.BuildVariants {
			buildVariants = append(buildVariants, buildVariant.Name)
		}

		projectNameToBuildVariants[projectName] = buildVariants
	}

	return projectNameToBuildVariants, nil
}

// construct the change information
// struct from a given version struct
func constructChangeInfo(v *version.Version, notification *NotificationKey) (changeInfo *ChangeInfo) {
	changeInfo = &ChangeInfo{}
	switch notification.NotificationRequester {
	case mci.RepotrackerVersionRequester:
		changeInfo.Project = v.Project
		changeInfo.Author = v.Author
		changeInfo.Message = v.Message
		changeInfo.Revision = v.Revision
		changeInfo.Email = v.AuthorEmail

	case mci.PatchVersionRequester:
		// get the author and description from the patch request
		patch, err := patch.FindOne(patch.ByVersion(v.Id))
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error finding patch for version %v: %v", v.Id, err)
			return
		}

		if patch == nil {
			mci.Logger.Errorf(slogger.ERROR, "%v notification was unable to locate patch with version: %v", notification, v.Id)
			return
		}
		// get the display name and email for this user
		dbUser, err := user.FindOne(user.ById(patch.Author))
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error finding user %v: %v",
				patch.Author, err)
			changeInfo.Author = patch.Author
			changeInfo.Email = patch.Author
		} else if dbUser == nil {
			mci.Logger.Errorf(slogger.ERROR, "User %v not found", patch.Author)
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

// use mail's rfc2047 to encode any string
func encodeRFC2047(String string) string {
	addr := mail.Address{String, ""}
	return strings.Trim(addr.String(), " <>")
}

// get the display name for a build variant's
// task given the build variant name
func getDisplayName(buildVariant string) (displayName string) {
	build, err := model.FindDisplayName(buildVariant)
	if err != nil || build == nil {
		mci.Logger.Errorf(slogger.ERROR, "Error fetching buildvariant name: %v", err)
		displayName = buildVariant
	} else {
		displayName = build.DisplayName
	}
	return
}

// get the failed task(s) for a given build
func getFailedTasks(current *model.Build, notificationName string) (failedTasks []model.TaskCache) {
	if util.SliceContains(buildFailureKeys, notificationName) {
		for _, task := range current.Tasks {
			if task.Status == mci.TaskFailed {
				failedTasks = append(failedTasks, task)
			}
		}
	}
	return
}

// get the specific failed test(s) for this task
func getFailedTests(current *model.Task, notificationName string) (failedTests []model.TestResult) {
	if util.SliceContains(taskFailureKeys, notificationName) {
		for _, test := range current.TestResults {
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
			return err
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
func getRecentlyFinishedBuilds(notificationKey *NotificationKey) (builds []model.Build, err error) {
	if cachedProjectRecords[notificationKey.String()] == nil {
		builds, err = model.RecentlyFinishedBuilds(lastProjectNotificationTime[notificationKey.Project], notificationKey.Project, notificationKey.NotificationRequester)
		if err != nil {
			return nil, err
		}
		cachedProjectRecords[notificationKey.String()] = builds
		return builds, err
	}
	return cachedProjectRecords[notificationKey.String()].([]model.Build), nil
}

// This is used to pull recently finished tasks
func getRecentlyFinishedTasks(notificationKey *NotificationKey) (tasks []model.Task, err error) {
	if cachedProjectRecords[notificationKey.String()] == nil {
		tasks, err = model.RecentlyFinishedTasks(lastProjectNotificationTime[notificationKey.Project], notificationKey.Project, notificationKey.NotificationRequester)
		if err != nil {
			return nil, err
		}
		cachedProjectRecords[notificationKey.String()] = tasks
		return tasks, err
	}
	return cachedProjectRecords[notificationKey.String()].([]model.Task), nil
}

// gets the type of notification - we support build/task level notification
func getType(notification string) (nkType string) {
	nkType = taskType
	if strings.Contains(notification, buildType) {
		nkType = buildType
	}
	return
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
				NotificationRequester: mci.RepotrackerVersionRequester,
			}

			// prevent duplicate notifications from being sent
			if !util.SliceContains(notifyOn, key) {
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
					NotificationRequester: mci.RepotrackerVersionRequester,
				}

				// prevent duplicate notifications from being sent
				if !util.SliceContains(notifyOn, key) {
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
				NotificationRequester: mci.PatchVersionRequester,
			}

			// prevent duplicate notifications from being sent
			if !util.SliceContains(notifyOn, key) {
				notifyOn = append(notifyOn, key)
			}
		}
	}
	return
}

// NotifyAdmins is a helper method to send a notification to the MCI admin team
func NotifyAdmins(subject, message string, mciSettings *mci.MCISettings) error {
	if mciSettings.Notify.SMTP != nil {
		return TrySendNotification(mciSettings.Notify.SMTP.AdminEmail,
			subject, message, ConstructMailer(mciSettings.Notify))
	}
	return mci.Logger.Errorf(slogger.ERROR, "Cannot notify admins: admin_email not set")
}

// String method for notification key
func (nk NotificationKey) String() string {
	return fmt.Sprintf("%v-%v-%v", nk.Project, nk.NotificationType, nk.NotificationRequester)
}

// Helper function to send notifications
func TrySendNotification(recipients []string, subject, body string, mailer Mailer) (err error) {
	// mci.Logger.Logf(slogger.DEBUG, "address: %v subject: %v body: %v", recipients, subject, body)
	// return nil
	_, err = util.RetryArithmeticBackoff(func() error {
		err = mailer.SendMail(recipients, subject, body)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending notification: %v", err)
			return util.RetriableError{err}
		}
		return nil
	}, NumSmtpRetries, SmtpSleepTime)
	return err
}

// Helper function to send notification to a given user
func TrySendNotificationToUser(userId string, subject, body string, mailer Mailer) (err error) {
	dbUser, err := user.FindOne(user.ById(userId))
	if err != nil {
		return fmt.Errorf("Error finding user %v: %v", userId, err)
	} else if dbUser == nil {
		return fmt.Errorf("User %v not found", userId)
	} else {
		return TrySendNotification([]string{dbUser.Email()}, subject, body, mailer)
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
