package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/apiserver"
	dbutil "10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/plugin"
	"10gen.com/mci/testutils"
	"10gen.com/mci/util"
	"crypto/tls"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

//Set this to "true" to see the full log output for all tests.
//If something is failing, try turning this on to see all the details
var testConfig = mci.TestConfig()

var Verbose = true

var testSetups = []testConfigPath{
	{"With plugin mode test config", "testdata/config_test_plugin"},
}

var buildVariantsToTest = []string{
	"linux-64",
	"windows8",
}

var tlsConfigs map[string]*tls.Config

// the revision we'll assume is the current one for the agent. this is the
// same as appears in testdata/executables/version
var agentRevision = "xxx"

//NoopSignal is a signal handler that ignores all signals, so that we can
//intercept and check for them directly in the test instead
type NoopSignalHandler struct{}

type testConfigPath struct {
	testSpec   string
	configPath string
}

func init() {
	dbutil.SetGlobalSessionProvider(dbutil.SessionFactoryFromConfig(testConfig))
}

func setupTlsConfigs(t *testing.T) {
	if tlsConfigs == nil {
		tlsConfig := &tls.Config{}
		tlsConfig.NextProtos = []string{"http/1.1"}

		var err error
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err =
			tls.X509KeyPair([]byte(testConfig.Expansions["api_httpscert"]),
				[]byte(testConfig.Api.HttpsKey))
		if err != nil {
			util.HandleTestingErr(err, t, "X509KeyPair failed during test initialization: %v", err)
		}
		tlsConfigs = map[string]*tls.Config{
			"http":  nil,
			"https": tlsConfig,
		}
	}
}

func (self *NoopSignalHandler) HandleSignals(taskCom TaskCommunicator,
	signalChannel chan AgentSignal, completed chan FinalTaskFunc) {
	return
}

func TestBasicEndpoints(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {

		testTask, _, err := setupAPITestData(testConfig, mci.CompileStage,
			"linux-64", false, t)
		util.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
			util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
				Verbose, testConfig.Expansions["api_httpscert"])
			util.HandleTestingErr(err, t, "Couldn't create agent: %v", err)
			testAgent.heartbeater.Interval = 10 * time.Second
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling start should flip the task's status to started", func() {
				testAgent.Start("1")
				testTask, err = model.FindTask(testTask.Id)
				util.HandleTestingErr(err, t, "Couldn't refresh task from db: %v", err)
				So(testTask.Status, ShouldEqual, mci.TaskStarted)

				testHost, err := host.FindOne(host.ByRunningTaskId(testTask.Id))
				So(err, ShouldBeNil)
				So(testHost.Id, ShouldEqual, "testHost")
				So(testHost.RunningTask, ShouldEqual, testTask.Id)
			})

			Convey("calling gettask should get retrieve same task back", func() {
				testTaskFromApi, err := testAgent.GetTask()
				So(err, ShouldBeNil)

				//ShouldResemble doesn't seem to work here, possibly because of
				//omitempty? anyways, just assert equality of the important fields
				So(testTaskFromApi.Id, ShouldEqual, testTask.Id)
				So(testTaskFromApi.Status, ShouldEqual, testTask.Status)
				So(testTaskFromApi.HostId, ShouldEqual, testTask.HostId)
			})

			Convey("sending logs should store the log messages properly", func() {
				msg1 := "task logger initialized!"
				msg2 := "system logger initialized!"
				msg3 := "exec logger initialized!"
				testAgent.agentLogger.LogTask(slogger.INFO, msg1)
				testAgent.agentLogger.LogSystem(slogger.INFO, msg2)
				testAgent.agentLogger.LogExecution(slogger.INFO, msg3)
				time.Sleep(100 * time.Millisecond)
				testAgent.RemoteAppender.FlushAndWait()

				//This returns logs in order of NEWEST first.
				logMessages, err :=
					model.FindMostRecentLogMessages(testTask.Id, 0, 10, []string{},
						[]string{})
				util.HandleTestingErr(err, t, "failed to get log msgs")

				So(logMessages[2].Message, ShouldEndWith, msg1)
				So(logMessages[1].Message, ShouldEndWith, msg2)
				So(logMessages[0].Message, ShouldEndWith, msg3)
			})

			Convey("calling end() should update task status properly", func() {
				testAgent.End(mci.TaskSucceeded, nil)
				time.Sleep(100 * time.Millisecond)
				taskUpdate, err := model.FindTask(testTask.Id)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, mci.TaskSucceeded)
			})

			Convey("no checkins should trigger timeout signal", func() {
				testAgent.timeoutWatcher.SetTimeoutDuration(2 * time.Second)
				testAgent.timeoutWatcher.CheckIn()
				//Sleep long enough for the timeout watcher to time out
				time.Sleep(3 * time.Second)
				timeoutSignal, ok := <-testAgent.signalChan
				So(ok, ShouldBeTrue)
				So(timeoutSignal, ShouldEqual, IdleTimeout)
			})
		})
	}
}

func TestHeartbeatSignals(t *testing.T) {
	setupTlsConfigs(t)

	for tlsString, tlsConfig := range tlsConfigs {

		testTask, _, err := setupAPITestData(testConfig, mci.CompileStage, "linux-64", false, t)
		util.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
			util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)

			testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
				Verbose, testConfig.Expansions["api_httpscert"])
			util.HandleTestingErr(err, t, "failed to get test agent")
			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				testServer.Listener.Close()
				signal, ok := <-testAgent.signalChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, HeartbeatMaxFailed)
			})
		})
	}
}

func TestSecrets(t *testing.T) {
	setupTlsConfigs(t)
	testTask, _, err := setupAPITestData(testConfig, mci.CompileStage,
		"linux-64", false, t)
	util.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
			util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)

			testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
				Verbose, testConfig.Expansions["api_httpscert"])
			util.HandleTestingErr(err, t, "Couldn't create agent: %v", err)

			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				testServer.Listener.Close()
				signal, ok := <-testAgent.signalChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, HeartbeatMaxFailed)
			})
		})
	}
}

func TestTaskSuccess(t *testing.T) {
	setupTlsConfigs(t)
	testutils.ConfigureIntegrationTest(t, testConfig, "TestTaskSuccess")

	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			for _, variant := range buildVariantsToTest {
				Convey(testSetup.testSpec, t, func() {
					configAbsPath, err := filepath.Abs(testSetup.configPath)
					util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)

					Convey("With agent running 'compile' step and live API server over "+
						tlsString+" with variant "+variant, func() {
						testTask, _, err := setupAPITestData(testConfig, mci.CompileStage,
							variant, false, t)
						util.HandleTestingErr(err, t, "Couldn't create test task: %v", err)
						testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
						util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
							Verbose, testConfig.Expansions["api_httpscert"])

						//actually run the task.
						//this function won't return until the whole thing is done.
						workDir, err := ioutil.TempDir("", "testtask_")
						util.HandleTestingErr(err, t, "Couldn't create work dir: %v", err)

						RunTask(testAgent, configAbsPath, workDir)
						Convey("expansions should be fetched", func() {
							So(testAgent.taskConfig.Expansions.Get("aws_key"), ShouldEqual, testConfig.Providers.AWS.Id)
							So(scanLogsForTask(testTask.Id, "fetch_expansion_value"), ShouldBeTrue)
						})
						time.Sleep(100 * time.Millisecond)
						testAgent.RemoteAppender.FlushAndWait()
						printLogsForTask(testTask.Id)

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "[shell.exec] Executing script: echo \"predefined command!\""), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "predefined command!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "this should not end up in the logs"), ShouldBeFalse)
							So(scanLogsForTask(testTask.Id, "Cloning into") || // git 1.8
								scanLogsForTask(testTask.Id, "Initialized empty Git repository"), // git 1.7
								ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "i am compiling!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "i am sanity testing!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Skipping command git.apply_patch on variant"), ShouldBeTrue)

							//Check that functions with args are working correctly
							So(scanLogsForTask(testTask.Id, "arg1 is FOO"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg2 is BAR"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg3 is Expanded: qux"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg4 is Default: default_value"), ShouldBeTrue)

							//Check that logging output is only flushing on a newline
							So(scanLogsForTask(testTask.Id, "this should be on the same line...as this."), ShouldBeTrue)

							testTask, err = model.FindTask(testTask.Id)
							util.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, mci.TaskSucceeded)
						})
					})

					Convey("With agent running a regular test and live API server over "+
						tlsString+" on variant "+variant, func() {
						testTask, _, err := setupAPITestData(testConfig, "normal_task",
							variant, false, t)
						util.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
						testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
						util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
							Verbose, testConfig.Expansions["api_httpscert"])

						//actually run the task.
						//this function won't return until the whole thing is done.
						workDir, err := ioutil.TempDir("", "testtask_")
						util.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
						RunTask(testAgent, configAbsPath, workDir)
						time.Sleep(100 * time.Millisecond)
						testAgent.RemoteAppender.FlushAndWait()

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

							So(scanLogsForTask(testTask.Id, "starting normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with normal_task!"), ShouldBeTrue)

							testTask, err = model.FindTask(testTask.Id)
							util.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, mci.TaskSucceeded)

							expectedResults := []model.TestResult{
								model.TestResult{
									Status:    "success",
									TestFile:  "t1",
									URL:       "url",
									ExitCode:  0,
									StartTime: 0,
									EndTime:   10,
								},
							}
							So(testTask.TestResults, ShouldResemble, expectedResults)
						})
					})
				})
			}
		}
	}
}

func TestTaskFailures(t *testing.T) {
	setupTlsConfigs(t)
	testutils.ConfigureIntegrationTest(t, testConfig, "TestTaskFailures")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				configAbsPath, err := filepath.Abs(testSetup.configPath)
				util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)
				Convey("With agent running a failing test and live API server over "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, "failing_task",
						"linux-64", false, t)
					util.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
					util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
						Verbose, testConfig.Expansions["api_httpscert"])

					//actually run the task.
					//this function won't return until the whole thing is done.
					workDir, err := ioutil.TempDir("", "testtask_")
					util.HandleTestingErr(err, t, "Couldn't create work dir: %v", err)
					RunTask(testAgent, configAbsPath, workDir)
					time.Sleep(100 * time.Millisecond)
					testAgent.RemoteAppender.FlushAndWait()
					printLogsForTask(testTask.Id)
					//time.Sleep(3 * time.Second)

					Convey("the pre and post-run scripts should have run", func() {
						So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

						Convey("the task should have run up until its first failure", func() {
							So(scanLogsForTask(testTask.Id, "starting failing_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with failing_task!"), ShouldBeFalse)
						})

						Convey("the tasks's final status should be FAILED", func() {
							testTask, err = model.FindTask(testTask.Id)
							util.HandleTestingErr(err, t, "Failed to find test task")
							So(testTask.Status, ShouldEqual, mci.TaskFailed)
						})
					})
				})
			})
		}
	}
}

func TestTaskAbortion(t *testing.T) {
	setupTlsConfigs(t)
	testutils.ConfigureIntegrationTest(t, testConfig, "TestTaskAbortion")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				configAbsPath, err := filepath.Abs(testSetup.configPath)
				util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)

				Convey("With agent running a slow test and live API server over "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, "very_slow_task",
						"linux-64", false, t)
					util.HandleTestingErr(err, t, "Failed to find test task")
					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
					util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
						Verbose, testConfig.Expansions["api_httpscert"])

					Convey("when the abort signal is triggered on the task", func() {
						go func() {
							//Wait for a few seconds, then switch the task to aborted!
							time.Sleep(3 * time.Second)
							err := testTask.Abort("", true)
							util.HandleTestingErr(err, t, "Failed to abort test task")
							fmt.Println("aborted task.")
						}()

						//actually run the task.
						//this function won't return until the whole thing is done.
						workDir, err := ioutil.TempDir("", "testtask_")
						util.HandleTestingErr(err, t, "Failed to create temp dir")
						RunTask(testAgent, configAbsPath, workDir)
						testAgent.RemoteAppender.Flush()
						time.Sleep(1 * time.Second)
						printLogsForTask(testTask.Id)

						Convey("the pre and post-run scripts should have run", func() {
							So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Received abort signal - stopping."), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with very_slow_task!"), ShouldBeFalse)
							testTask, err = model.FindTask(testTask.Id)
							util.HandleTestingErr(err, t, "Failed to find test task")
							So(testTask.Status, ShouldEqual, mci.TaskUndispatched)
						})
					})
				})
			})
		}
	}
}

func TestTaskTimeout(t *testing.T) {
	setupTlsConfigs(t)
	configAbsPath, err := filepath.Abs("testdata/config_test_plugin")
	util.HandleTestingErr(err, t, "Couldn't get abs path for config: %v", err)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "timeout_task", "linux-64",
				false, t)
			util.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
			util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
				Verbose, testConfig.Expansions["api_httpscert"])
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			Convey("after the slow test runs beyond the timeout threshold", func() {
				//actually run the task.
				//this function won't return until the whole thing is done.
				workDir, err := ioutil.TempDir("", "testtask_")
				util.HandleTestingErr(err, t, "Failed to create temp dir")
				RunTask(testAgent, configAbsPath, workDir)
				testAgent.RemoteAppender.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "executing the pre-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = model.FindTask(testTask.Id)
					So(testTask.Status, ShouldEqual, mci.TaskFailed)
					So(testTask.StatusDetails.TimedOut, ShouldBeTrue)
				})
			})
		})
	}
}

func TestTaskEndEndpoint(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		testTask, _, err := setupAPITestData(testConfig, mci.CompileStage,
			"linux-64", false, t)
		util.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.Published, Verbose)
			util.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := NewAgent(testServer.URL, testTask.Id, testTask.Secret,
				Verbose, testConfig.Expansions["api_httpscert"])
			util.HandleTestingErr(err, t, "Couldn't create agent: %v", err)
			testAgent.heartbeater.Interval = 10 * time.Second
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling end() should update task's/host's status properly "+
				"and start running the next task", func() {
				subsequentTaskId := testTask.Id + "Two"
				taskEndResp, err := testAgent.End(mci.TaskSucceeded, nil)
				time.Sleep(1 * time.Second)
				So(err, ShouldBeNil)

				taskUpdate, err := model.FindTask(testTask.Id)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, mci.TaskSucceeded)

				testHost, err := host.FindOne(host.ById(testTask.HostId))
				So(err, ShouldBeNil)
				So(testHost.RunningTask, ShouldEqual, subsequentTaskId)

				taskUpdate, err = model.FindTask(subsequentTaskId)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, mci.TaskDispatched)

				So(taskEndResp, ShouldNotBeNil)
				So(taskEndResp.RunNext, ShouldBeTrue)
				So(taskEndResp.TaskId, ShouldEqual, subsequentTaskId)
			})
		})
	}
}

func scanLogsForTask(taskId string, scanFor string) bool {
	taskLogs, err := model.FindAllTaskLogs(taskId, 0)
	if err != nil {
		panic(err)
	}
	for _, taskLogObj := range taskLogs {
		for _, logmsg := range taskLogObj.Messages {
			if strings.Contains(logmsg.Message, scanFor) {
				return true
			}
		}
	}
	return false
}

func printLogsForTask(taskId string) {
	if !Verbose {
		return
	}
	logMessages, err := model.FindMostRecentLogMessages(taskId, 0, 100,
		[]string{}, []string{})
	if err != nil {
		panic(err)
		return
	}
	for i := len(logMessages) - 1; i >= 0; i-- {
		if logMessages[i].Type == model.SystemLogPrefix {
			continue
		}
		fmt.Println(logMessages[i].Message)
	}
}

func setupAPITestData(testConfig *mci.MCISettings, taskDisplayName string,
	variant string, patch bool, t *testing.T) (*model.Task, *model.Build, error) {
	//ignore errs here because the ns might just not exist.
	clearDataMsg := "Failed to clear test data collection"
	testCollections := []string{
		model.TasksCollection, model.BuildsCollection, host.Collection,
		distro.Collection, model.VersionsCollection, model.PatchCollection,
		model.PushlogCollection, model.ProjectVarsCollection, model.TaskQueuesCollection,
	}
	util.HandleTestingErr(dbutil.ClearCollections(testCollections...), t, clearDataMsg)
	projectVars := &model.ProjectVars{
		Id: "mongodb-mongo-master",
		Vars: map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"fetch_key":  "fetch_expansion_value",
		},
	}
	_, err := projectVars.Upsert()
	util.HandleTestingErr(err, t, clearDataMsg)

	taskOne := &model.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       mci.TaskDispatched,
		Requester:    mci.RepotrackerVersionRequester,
	}

	taskTwo := &model.Task{
		Id:           "testTaskIdTwo",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "",
		Secret:       "testTaskSecret",
		Activated:    true,
		Version:      "testVersionId",
		Status:       mci.TaskUndispatched,
		Requester:    mci.RepotrackerVersionRequester,
	}

	if patch {
		taskOne.Requester = mci.PatchVersionRequester
	}

	util.HandleTestingErr(taskOne.Insert(), t, "failed to insert taskOne")
	util.HandleTestingErr(taskTwo.Insert(), t, "failed to insert taskTwo")

	// set up task queue for task end tests
	taskQueue := &model.TaskQueue{
		Distro: "test-distro-one",
		Queue: []model.TaskQueueItem{
			model.TaskQueueItem{
				Id:          "testTaskIdTwo",
				DisplayName: taskDisplayName,
			},
		},
	}
	util.HandleTestingErr(taskQueue.Save(), t, "failed to insert taskqueue")

	host := &host.Host{
		Id:            "testHost",
		Host:          "testHost",
		RunningTask:   "testTaskId",
		StartedBy:     mci.MCIUser,
		AgentRevision: agentRevision,
	}
	util.HandleTestingErr(host.Insert(), t, "failed to insert host")

	// read in the project configuration
	projectConfig, err := ioutil.ReadFile("testdata/config_test_plugin/" +
		"project/mongodb-mongo-master.yml")
	util.HandleTestingErr(err, t, "failed to read project config")

	// unmarshall the project configuration into a struct
	project := &model.Project{}
	util.HandleTestingErr(yaml.Unmarshal(projectConfig, project), t,
		"failed to unmarshall project config")

	// now then marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(project)
	util.HandleTestingErr(err, t, "failed to marshall project config")

	// insert the version document
	version := &model.Version{
		Id:       "testVersionId",
		BuildIds: []string{taskOne.BuildId},
		Config:   string(projectYamlBytes),
	}

	util.HandleTestingErr(version.Insert(), t, "failed to insert version")
	if patch {
		mainPatchContent, err := ioutil.ReadFile("testdata/test.patch")
		util.HandleTestingErr(err, t, "failed to read test patch file")
		modulePatchContent, err := ioutil.ReadFile("testdata/testmodule.patch")
		util.HandleTestingErr(err, t, "failed to read test module patch file")

		patch := &model.Patch{
			Status:  mci.PatchCreated,
			Version: version.Id,
			Patches: []model.ModulePatch{
				model.ModulePatch{
					ModuleName: "",
					Githash:    "d0c52298b222f4973c48e9834a57966c448547de",
					PatchSet:   model.PatchSet{Patch: string(mainPatchContent)},
				},
				model.ModulePatch{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   model.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}
		util.HandleTestingErr(patch.Insert(), t, "failed to insert patch")
	}

	session, _, err := dbutil.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "couldn't get db session!")

	//Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).
		RemoveAll(bson.M{"t_id": bson.M{"$in": []string{taskOne.Id, taskTwo.Id}}})
	util.HandleTestingErr(err, t, "failed to remove logs")

	build := &model.Build{
		Id: "testBuildId",
		Tasks: []model.TaskCache{
			model.NewTaskCache(taskOne.Id, taskOne.DisplayName, true),
			model.NewTaskCache(taskTwo.Id, taskTwo.DisplayName, true),
		},
		Version: "testVersionId",
	}

	util.HandleTestingErr(build.Insert(), t, "failed to insert build")
	return taskOne, build, nil
}
