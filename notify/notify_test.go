package notify

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
	"time"
)

var (
	buildId       = "build"
	taskId        = "task"
	projectId     = "project"
	buildVariant  = "buildVariant"
	displayName   = "displayName"
	emailSubjects = make([]string, 0)
	emailBodies   = make([]string, 0)

	buildFailureNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      buildFailureKey,
		NotificationType:      buildType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	buildSucceessNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      buildSuccessKey,
		NotificationType:      buildType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	buildCompletionNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      buildCompletionKey,
		NotificationType:      buildType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	buildSuccessToFailureNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      buildSuccessToFailureKey,
		NotificationType:      buildType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	taskFailureNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      taskFailureKey,
		NotificationType:      taskType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	taskSucceessNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      taskSuccessKey,
		NotificationType:      taskType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	taskCompletionNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      taskCompletionKey,
		NotificationType:      taskType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	taskSuccessToFailureNotificationKey = NotificationKey{
		Project:               projectId,
		NotificationName:      taskSuccessToFailureKey,
		NotificationType:      taskType,
		NotificationRequester: evergreen.RepotrackerVersionRequester,
	}
	allNotificationKeys = []NotificationKey{
		buildFailureNotificationKey,
		buildSucceessNotificationKey,
		buildCompletionNotificationKey,
		buildSuccessToFailureNotificationKey,
		taskFailureNotificationKey,
		taskSucceessNotificationKey,
		taskCompletionNotificationKey,
		taskSuccessToFailureNotificationKey,
	}
)

var TestConfig = evergreen.TestConfig()

func TestNotify(t *testing.T) {
	if evergreen.TestConfig().Notify.LogFile != "" {
		evergreen.SetLogger(evergreen.TestConfig().Notify.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(TestConfig))
	emailSubjects = make([]string, 0)
	emailBodies = make([]string, 0)

	Convey("When running notification handlers", t, func() {

		ae, err := createEnvironment(TestConfig, map[string]interface{}{})
		So(err, ShouldBeNil)

		Convey("Build-specific handlers should return the correct emails", func() {
			cleanupdb()
			timeNow := time.Now()
			// insert the test documents
			insertBuildDocs(timeNow)
			version := &version.Version{Id: "version"}
			So(version.Insert(), ShouldBeNil)
			Convey("BuildFailureHandler should return 1 email per failed build", func() {
				handler := BuildFailureHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&buildFailureNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 2 failed notifications
				So(len(emails), ShouldEqual, 2)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] Build #build1 failed on displayName")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] Build #build9 failed on displayName")
			})

			Convey("BuildSuccessHandler should return 1 email per successful build", func() {
				handler := BuildSuccessHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&buildSucceessNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 2 success notifications
				So(len(emails), ShouldEqual, 2)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-SUCCESS ] Build #build3 succeeded on displayName")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-SUCCESS ] Build #build8 succeeded on displayName")
			})

			Convey("BuildCompletionHandler should return 1 email per completed build", func() {
				handler := BuildCompletionHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&buildCompletionNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 6 completed notifications
				So(len(emails), ShouldEqual, 6)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build1 completed on displayName")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build3 completed on displayName")
				So(emails[2].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build4 completed on displayName")
				So(emails[3].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build8 completed on displayName")
				So(emails[4].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build9 completed on displayName")
				So(emails[5].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] Build #build10 completed on displayName")
			})

			Convey("BuildSuccessToFailureHandler should return 1 email per "+
				"build success to failure transition", func() {
				handler := BuildSuccessToFailureHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&buildSuccessToFailureNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 1 success_to_failure notifications
				So(len(emails), ShouldEqual, 1)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] Build #build9 transitioned to failure on displayName")
			})
		})

		Convey("Task-specific handlers should return the correct emails", func() {
			cleanupdb()
			timeNow := time.Now()
			// insert the test documents
			insertTaskDocs(timeNow)
			v := &version.Version{Id: "version"}
			So(v.Insert(), ShouldBeNil)

			Convey("TaskFailureHandler should return 1 email per task failure", func() {
				handler := TaskFailureHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&taskFailureNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 2 failed notifications
				So(len(emails), ShouldEqual, 2)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] possible MCI failure in displayName (failed on build1)")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] possible MCI failure in displayName (failed on build1)")
			})

			Convey("TaskSuccessHandler should return 1 email per task success", func() {
				handler := TaskSuccessHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&taskSucceessNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 2 success notifications
				So(len(emails), ShouldEqual, 2)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-SUCCESS ] possible MCI failure in displayName (succeeded on build1)")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-SUCCESS ] possible MCI failure in displayName (succeeded on build1)")
			})

			Convey("TaskCompletionHandler should return 1 email per completed task", func() {
				handler := TaskCompletionHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&taskCompletionNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 6 completion notifications
				So(len(emails), ShouldEqual, 6)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
				So(emails[1].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
				So(emails[2].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
				So(emails[3].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
				So(emails[4].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
				So(emails[5].GetSubject(), ShouldEqual,
					"[MCI-COMPLETION ] possible MCI failure in displayName (completed on build1)")
			})

			Convey("TaskSuccessToFailureHandler should return 1 email per "+
				"task success to failure transition", func() {
				handler := TaskSuccessToFailureHandler{}
				emails, err := handler.GetNotifications(ae, "config_test",
					&taskSuccessToFailureNotificationKey)
				So(err, ShouldBeNil)
				// check that we only returned 1 success to failure notifications
				So(len(emails), ShouldEqual, 1)
				So(emails[0].GetSubject(), ShouldEqual,
					"[MCI-FAILURE ] possible MCI failure in displayName (transitioned to "+
						"failure on build1)")
			})
		})
	})

	Convey("When running notifications pipeline", t, func() {
		cleanupdb()
		timeNow := time.Now()
		// insert the test documents
		insertTaskDocs(timeNow)
		v := &version.Version{Id: "version"}
		So(v.Insert(), ShouldBeNil)

		Convey("Should run the correct notification handlers for given "+
			"notification keys", func() {
			notificationSettings := &MCINotification{}
			notificationSettings.Notifications = []Notification{
				Notification{"task_failure", "project", []string{"user@mongodb"}, []string{}},
				Notification{"task_success_to_failure", "project", []string{"user@mongodb"}, []string{}},
			}
			notificationSettings.Teams = []Team{
				Team{
					"myteam",
					"myteam@me.com",
					[]Subscription{Subscription{"task", []string{}, []string{"task_failure"}}},
				},
			}
			notificationSettings.PatchNotifications = []Subscription{
				Subscription{"patch_project", []string{}, []string{}},
			}

			notificationKeyFailure := NotificationKey{"project", "task_failure", "task", "gitter_request"}
			notificationKeyToFailure := NotificationKey{"project", "task_success_to_failure", "task",
				"gitter_request"}

			ae, err := createEnvironment(TestConfig, map[string]interface{}{})
			So(err, ShouldBeNil)

			emails, err := ProcessNotifications(ae, "config_test", notificationSettings, false)
			So(err, ShouldBeNil)

			So(len(emails[notificationKeyFailure]), ShouldEqual, 2)
			So(emails[notificationKeyFailure][0].GetSubject(), ShouldEqual,
				"[MCI-FAILURE ] possible MCI failure in displayName (failed on build1)")
			So(emails[notificationKeyFailure][1].GetSubject(), ShouldEqual,
				"[MCI-FAILURE ] possible MCI failure in displayName (failed on build1)")

			So(len(emails[notificationKeyToFailure]), ShouldEqual, 1)
			So(emails[notificationKeyToFailure][0].GetSubject(), ShouldEqual,
				"[MCI-FAILURE ] possible MCI failure in displayName (transitioned to "+
					"failure on build1)")
		})

		Convey("SendNotifications should send emails correctly", func() {
			notificationSettings := &MCINotification{}
			notificationSettings.Notifications = []Notification{
				Notification{"task_failure", "project", []string{"user@mongodb"}, []string{}},
			}
			notificationSettings.Teams = []Team{
				Team{
					"myteam",
					"myteam@me.com",
					[]Subscription{Subscription{"task", []string{}, []string{"task_failure"}}},
				},
			}
			notificationSettings.PatchNotifications = []Subscription{
				Subscription{"patch_project", []string{}, []string{}},
			}

			fakeTask, err := model.FindOneTask(bson.M{"_id": "task8"}, bson.M{}, []string{})

			notificationKey := NotificationKey{"project", "task_failure", "task", "gitter_request"}

			triggeredNotification := TriggeredTaskNotification{
				fakeTask,
				nil,
				[]ChangeInfo{},
				notificationKey,
				"[MCI-FAILURE]",
				"failed",
			}

			email := TaskEmail{
				EmailBase{
					"This is the email body",
					"This is the email subject",
					triggeredNotification.Info,
				},
				triggeredNotification,
			}

			m := make(map[NotificationKey][]Email)
			m[notificationKey] = []Email{&email}

			mailer := MockMailer{}
			mockSettings := evergreen.Settings{Notify: evergreen.NotifyConfig{}}
			err = SendNotifications(&mockSettings, notificationSettings, m, mailer)
			So(err, ShouldBeNil)

			So(len(emailSubjects), ShouldEqual, 1)
			So(emailSubjects[0], ShouldEqual,
				"This is the email subject")
			So(emailBodies[0], ShouldEqual,
				"This is the email body")
		})
	})
}

func insertBuildDocs(priorTime time.Time) {
	// add test build docs to the build collection

	// build 1
	// build finished unsuccessfully (failed)
	// should trigger the following handler(s):
	// - buildFailureHandler
	// - buildCompletionHandler
	insertBuild(buildId+"1", projectId, displayName, buildVariant,
		evergreen.BuildFailed, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 1)

	// build 2
	// build not finished
	insertBuild(buildId+"2", projectId, displayName, buildVariant,
		evergreen.BuildStarted, time.Now(), time.Now(), time.Duration(0), true,
		evergreen.RepotrackerVersionRequester, 2)

	// build 3
	// build finished successfully (success)
	// should trigger the following handler(s):
	// - buildSuccessHandler
	// - buildCompletionHandler
	insertBuild(buildId+"3", projectId, displayName, buildVariant,
		evergreen.BuildSucceeded, time.Now(), time.Now(), time.Duration(50), true,
		evergreen.RepotrackerVersionRequester, 3)

	// build 4
	// build cancelled (cancelled)
	// - buildCompletionHandler
	insertBuild(buildId+"4", projectId, displayName, buildVariant,
		evergreen.BuildCancelled, time.Now(), priorTime, time.Duration(50), true,
		evergreen.RepotrackerVersionRequester, 4)

	// build 5
	// build not finished
	insertBuild(buildId+"5", projectId, displayName, buildVariant,
		evergreen.BuildStarted, time.Now(), time.Now(), time.Duration(0), true,
		evergreen.RepotrackerVersionRequester, 5)

	// build 6
	// build finished (failed) from different project
	// should trigger the following handler(s):
	// - buildFailureHandler
	// - buildCompletionHandler
	insertBuild(buildId+"6", projectId+"_", displayName, buildVariant,
		evergreen.BuildFailed, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 6)

	// build 7
	// build finished (succeeded) from different project
	// should trigger the following handler(s):
	// - buildSuccessHandler
	insertBuild(buildId+"7", projectId+"_", displayName, buildVariant,
		evergreen.BuildSucceeded, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 7)

	// build 8
	// build finished (succeeded) from different build variant
	// should trigger the following handler(s):
	// - buildSuccessToFailureHandler (in conjunction with 9)
	// - buildCompletionHandler
	insertBuild(buildId+"8", projectId, displayName, buildVariant+"_",
		evergreen.BuildSucceeded, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 8)

	// build 9
	// build finished (failed) from different build variant
	// should trigger the following handler(s):
	// - buildSuccessToFailureHandler (in conjunction with 8)
	// - buildCompletionHandler
	insertBuild(buildId+"9", projectId, displayName, buildVariant+"_",
		evergreen.BuildFailed, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 9)

	// build 10
	// build finished (cancelled) from different build variant
	insertBuild(buildId+"10", projectId, displayName, buildVariant+"_",
		evergreen.BuildCancelled, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 10)

	insertVersions()
}

func insertTaskDocs(priorTime time.Time) {
	// add test task docs to the task collection

	// task 1
	// task finished unsuccessfully (failed)
	// should trigger the following handler(s):
	// - taskFailureHandler
	// - taskCompletionHandler
	insertTask(taskId+"1", projectId, displayName, buildVariant, evergreen.TaskFailed,
		time.Now(), time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 1)

	// task 2
	// task not finished
	insertTask(taskId+"2", projectId, displayName, buildVariant, evergreen.TaskStarted,
		time.Now(), time.Now(), time.Now(), time.Duration(0), true,
		evergreen.RepotrackerVersionRequester, 2)

	// task 3
	// task finished successfully (success)
	// should trigger the following handler(s):
	// - taskSuccessHandler
	// - taskCompletionHandler
	insertTask(taskId+"3", projectId, displayName, buildVariant,
		evergreen.TaskSucceeded, time.Now(), time.Now(), time.Now(), time.Duration(50),
		true, evergreen.RepotrackerVersionRequester, 3)

	// task 4
	// task cancelled (cancelled)
	// should trigger the following handler(s):
	// - taskCompletionHandler
	insertTask(taskId+"4", projectId, displayName, buildVariant,
		evergreen.TaskCancelled, time.Now(), time.Now(), priorTime, time.Duration(50),
		true, evergreen.RepotrackerVersionRequester, 4)

	// task 5
	// task not finished
	insertTask(taskId+"5", projectId, displayName, buildVariant, evergreen.TaskStarted,
		time.Now(), time.Now(), time.Now(), time.Duration(0), true,
		evergreen.RepotrackerVersionRequester, 5)

	// task 6
	// task finished (failed) from different project
	insertTask(taskId+"6", projectId+"_", displayName, buildVariant,
		evergreen.TaskFailed, time.Now(), time.Now(), time.Now(), time.Duration(10),
		true, evergreen.RepotrackerVersionRequester, 6)

	// task 7
	// task finished (succeeded) from different project
	insertTask(taskId+"7", projectId+"_", displayName, buildVariant,
		evergreen.TaskSucceeded, time.Now(), time.Now(), time.Now(), time.Duration(10),
		true, evergreen.RepotrackerVersionRequester, 7)

	// task 8
	// task finished (succeeded) from different build variant
	// should trigger the following handler(s):
	// - taskSuccessHandler
	// - taskCompletionHandler
	// - taskSuccessToFailureHandler (in conjunction with 9)
	insertTask(taskId+"8", projectId, displayName, buildVariant+"_",
		evergreen.TaskSucceeded, time.Now(), time.Now(), time.Now(), time.Duration(10),
		true, evergreen.RepotrackerVersionRequester, 8)

	// task 9
	// task finished (failed) from different build variant
	// should trigger the following handler(s):
	// - taskFailedHandler
	// - taskCompletionHandler
	// - taskSuccessToFailureHandler (in conjunction with 8)
	insertTask(taskId+"9", projectId, displayName, buildVariant+"_",
		evergreen.TaskFailed, time.Now(), time.Now(),
		time.Now().Add(time.Duration(3*time.Minute)), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 9)

	// task 10
	// task finished (cancelled) from different build variant
	// should trigger the following handler(s):
	// - taskCompletionHandler
	insertTask(taskId+"10", projectId, displayName, buildVariant+"_",
		evergreen.TaskCancelled, time.Now(), time.Now(),
		time.Now().Add(time.Duration(-1*time.Minute)), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 10)

	insertVersions()

	insertBuild(buildId+"0", projectId, displayName, buildVariant+"_",
		evergreen.BuildCancelled, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 10)

	insertBuild(buildId+"1", projectId, displayName, buildVariant,
		evergreen.BuildCancelled, time.Now(), time.Now(), time.Duration(10), true,
		evergreen.RepotrackerVersionRequester, 10)
}

func insertBuild(id, project, display_name, buildVariant, status string, createTime,
	finishTime time.Time, timeTaken time.Duration, activated bool, requester string,
	order int) {
	build := &build.Build{
		Id:                  id,
		BuildNumber:         id,
		Project:             project,
		BuildVariant:        buildVariant,
		TimeTaken:           timeTaken,
		Status:              status,
		CreateTime:          createTime,
		DisplayName:         display_name,
		FinishTime:          finishTime,
		Activated:           activated,
		Requester:           requester,
		RevisionOrderNumber: order,
		Version:             "version",
	}
	So(build.Insert(), ShouldBeNil)
}

func insertTask(id, project, display_name, buildVariant, status string, createTime,
	finishTime, pushTime time.Time, timeTaken time.Duration, activated bool,
	requester string, order int) {
	task := &model.Task{
		Id:                  id,
		Project:             project,
		DisplayName:         display_name,
		Status:              status,
		BuildVariant:        buildVariant,
		CreateTime:          createTime,
		PushTime:            pushTime,
		FinishTime:          finishTime,
		TimeTaken:           timeTaken,
		Activated:           activated,
		Requester:           requester,
		RevisionOrderNumber: order,
		BuildId:             "build1",
		Version:             "version",
	}
	So(task.Insert(), ShouldBeNil)
}

func insertVersions() {
	v := &version.Version{
		Id:       "version1",
		Project:  "",
		BuildIds: []string{"build1"},
		Author:   "user@mci",
		Message:  "Fixed all the bugs",
	}

	So(v.Insert(), ShouldBeNil)

	version2 := &version.Version{
		Id:      "version2",
		Project: "",
		BuildIds: []string{"build2", "build3", "build4", "build5", "build6",
			"build7", "build8", "build9", "build10"},
		Author:  "user@mci",
		Message: "Fixed all the other bugs",
	}

	So(version2.Insert(), ShouldBeNil)
}

func cleanupdb() {
	err := db.ClearCollections(
		model.TasksCollection,
		model.NotifyTimesCollection,
		model.NotifyHistoryCollection,
		build.Collection,
		version.Collection)
	So(err, ShouldBeNil)
}

type MockMailer struct{}

func (self MockMailer) SendMail(recipients []string, subject, body string) error {
	emailSubjects = append(emailSubjects, subject)
	emailBodies = append(emailBodies, body)
	return nil
}
