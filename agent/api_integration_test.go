package agent

import (
	"crypto/tls"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/command"
	dbutil "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

// Set this to "true" to see the full log output for all tests.
// If something is failing, try turning this on to see all the details
var Verbose = testing.Verbose()

var testConfig = evergreen.TestConfig()

var testSetups = []testConfigPath{
	{"With plugin mode test config", "testdata/config_test_plugin"},
}

var buildVariantsToTest = []string{"linux-64", "windows8"}

var tlsConfigs map[string]*tls.Config

// the revision we'll assume is the current one for the agent. this is the
// same as appears in testdata/executables/version
var agentRevision = "xxx"

// NoopSignal is a signal handler that ignores all signals, so that we can
// intercept and check for them directly in the test instead
type NoopSignalHandler struct{}

type testConfigPath struct {
	testSpec   string
	configPath string
}

type patchTestMode int

const (
	NoPatch patchTestMode = iota
	InlinePatch
	ExternalPatch
)

func (ptm patchTestMode) String() string {
	switch ptm {
	case NoPatch:
		return "none"
	case InlinePatch:
		return "inline"
	case ExternalPatch:
		return "external"
	}
	return "unknown"
}

func init() {
	dbutil.SetGlobalSessionProvider(dbutil.SessionFactoryFromConfig(testConfig))
}

func (*NoopSignalHandler) HandleSignals(_ *Agent) {
	return
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
			testutil.HandleTestingErr(err, t, "X509KeyPair failed during test initialization: %v", err)
		}
		tlsConfigs = map[string]*tls.Config{
			"http":  nil,
			"https": tlsConfig,
		}
	}
}

func createAgent(testServer *apiserver.TestServer, testTask *model.Task) (*Agent, error) {
	testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Expansions["api_httpscert"])
	if err != nil {
		return nil, err
	}
	testAgent.heartbeater.Interval = 10 * time.Second
	testAgent.taskConfig = &model.TaskConfig{Expansions: &command.Expansions{}}
	return testAgent, nil
}

func TestBasicEndpoints(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		testTask, _, err := setupAPITestData(testConfig, "task", "linux-64", NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := createAgent(testServer, testTask)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling GetTask should get retrieve same task back", func() {
				testTaskFromApi, err := testAgent.GetTask()
				So(err, ShouldBeNil)

				// ShouldResemble doesn't seem to work here, possibly because of
				// omitempty? anyways, just assert equality of the important fields
				So(testTaskFromApi.Id, ShouldEqual, testTask.Id)
				So(testTaskFromApi.Status, ShouldEqual, testTask.Status)
				So(testTaskFromApi.HostId, ShouldEqual, testTask.HostId)
			})

			Convey("calling start should flip the task's status to started", func() {
				err := testAgent.Start("1")
				testutil.HandleTestingErr(err, t, "Couldn't start task: %v", err)
				testTask, err := model.FindTask(testTask.Id)
				testutil.HandleTestingErr(err, t, "Couldn't refresh task from db: %v", err)
				So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
				testHost, err := host.FindOne(host.ByRunningTaskId(testTask.Id))
				So(err, ShouldBeNil)
				So(testHost.Id, ShouldEqual, "testHost")
				So(testHost.RunningTask, ShouldEqual, testTask.Id)
			})
			Convey("sending logs should store the log messages properly", func() {
				msg1 := "task logger initialized!"
				msg2 := "system logger initialized!"
				msg3 := "exec logger initialized!"
				testAgent.logger.LogTask(slogger.INFO, msg1)
				testAgent.logger.LogSystem(slogger.INFO, msg2)
				testAgent.logger.LogExecution(slogger.INFO, msg3)
				time.Sleep(100 * time.Millisecond)
				testAgent.APILogger.FlushAndWait()

				// This returns logs in order of NEWEST first.
				logMessages, err := model.FindMostRecentLogMessages(testTask.Id, 0, 10, []string{}, []string{})
				testutil.HandleTestingErr(err, t, "failed to get log msgs")

				So(logMessages[2].Message, ShouldEndWith, msg1)
				So(logMessages[1].Message, ShouldEndWith, msg2)
				So(logMessages[0].Message, ShouldEndWith, msg3)
			})

			Convey("calling end() should update task status properly", func() {
				commandType := model.SystemCommandType
				description := "random"
				details := &apimodels.TaskEndDetail{
					Description: description,
					Type:        commandType,
					TimedOut:    true,
					Status:      evergreen.TaskSucceeded,
				}
				testAgent.End(details)
				time.Sleep(100 * time.Millisecond)
				taskUpdate, err := model.FindTask(testTask.Id)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, evergreen.TaskSucceeded)
				So(taskUpdate.Details.Description, ShouldEqual, description)
				So(taskUpdate.Details.Type, ShouldEqual, commandType)
				So(taskUpdate.Details.TimedOut, ShouldEqual, true)
			})

			Convey("no checkins should trigger timeout signal", func() {
				testAgent.idleTimeoutWatcher.SetDuration(2 * time.Second)
				testAgent.idleTimeoutWatcher.CheckIn()
				// sleep long enough for the timeout watcher to time out
				time.Sleep(3 * time.Second)
				timeoutSignal, ok := <-testAgent.signalHandler.idleTimeoutChan
				So(ok, ShouldBeTrue)
				So(timeoutSignal, ShouldEqual, IdleTimeout)
			})
		})
	}
}

func TestHeartbeatSignals(t *testing.T) {
	setupTlsConfigs(t)

	for tlsString, tlsConfig := range tlsConfigs {

		testTask, _, err := setupAPITestData(testConfig, evergreen.CompileStage, "linux-64", NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := createAgent(testServer, testTask)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				testServer.Listener.Close()
				signal, ok := <-testAgent.signalHandler.heartbeatChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, HeartbeatMaxFailed)
			})
		})
	}
}

func TestSecrets(t *testing.T) {
	setupTlsConfigs(t)
	testTask, _, err := setupAPITestData(testConfig, evergreen.CompileStage, "linux-64", NoPatch, t)
	testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := createAgent(testServer, testTask)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				testServer.Listener.Close()
				signal, ok := <-testAgent.signalHandler.heartbeatChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, HeartbeatMaxFailed)
			})
		})
	}
}

func TestTaskSuccess(t *testing.T) {
	setupTlsConfigs(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskSuccess")

	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			for _, variant := range buildVariantsToTest {
				Convey(testSetup.testSpec, t, func() {
					Convey("With agent running 'compile' step and live API server over "+
						tlsString+" with variant "+variant, func() {
						testTask, _, err := setupAPITestData(testConfig, "compile", variant, NoPatch, t)
						testutil.HandleTestingErr(err, t, "Couldn't create test task: %v", err)
						testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						testAgent, err := createAgent(testServer, testTask)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")

						// actually run the task.
						// this function won't return until the whole thing is done.
						testAgent.RunTask()
						Convey("expansions should be fetched", func() {
							So(testAgent.taskConfig.Expansions.Get("aws_key"), ShouldEqual, testConfig.Providers.AWS.Id)
							So(scanLogsForTask(testTask.Id, "fetch_expansion_value"), ShouldBeTrue)
						})
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()
						printLogsForTask(testTask.Id)

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "Executing script: echo \"predefined command!\""), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "predefined command!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "this should not end up in the logs"), ShouldBeFalse)
							So(scanLogsForTask(testTask.Id, "Command timeout set to 21m40s"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Command timeout set to 43m20s"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Cloning into") || // git 1.8
								scanLogsForTask(testTask.Id, "Initialized empty Git repository"), // git 1.7
								ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "i am compiling!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "i am sanity testing!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Skipping command 'git.apply_patch' on variant"), ShouldBeTrue)

							// Check that functions with args are working correctly
							So(scanLogsForTask(testTask.Id, "arg1 is FOO"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg2 is BAR"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg3 is Expanded: qux"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "arg4 is Default: default_value"), ShouldBeTrue)

							// Check that multi-command functions are working correctly
							So(scanLogsForTask(testTask.Id, "step 1 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "step 2 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "step 3 of multi-command func"), ShouldBeTrue)

							// Check that logging output is only flushing on a newline
							So(scanLogsForTask(testTask.Id, "this should be on the same line...as this."), ShouldBeTrue)

							testTask, err = model.FindTask(testTask.Id)
							testutil.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)

							// use function display name as description when none is specified in command
							So(testTask.Details.Status, ShouldEqual, evergreen.TaskSucceeded)
							So(testTask.Details.Description, ShouldEqual, `'shell.exec' in "silent shell test"`)
							So(testTask.Details.TimedOut, ShouldBeFalse)
							So(testTask.Details.Type, ShouldEqual, model.SystemCommandType)
						})

						Convey("manifest should be created", func() {
							m, err := manifest.FindOne(manifest.ById(testTask.Version))
							So(testTask.Version, ShouldEqual, "testVersionId")
							So(err, ShouldBeNil)
							So(m, ShouldNotBeNil)
							So(m.ProjectName, ShouldEqual, testTask.Project)
							So(m.Id, ShouldEqual, testTask.Version)
							So(m.Modules, ShouldNotBeEmpty)
							So(m.Modules["recursive"], ShouldNotBeNil)
							So(m.Modules["recursive"].URL, ShouldNotEqual, "")
						})
					})

					Convey("With agent running a regular test and live API server over "+
						tlsString+" on variant "+variant, func() {
						testTask, _, err := setupAPITestData(testConfig, "normal_task", variant, NoPatch, t)
						testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
						testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						testAgent, err := createAgent(testServer, testTask)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")

						// actually run the task.
						// this function won't return until the whole thing is done.
						testAgent.RunTask()
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

							So(scanLogsForTask(testTask.Id, "starting normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with normal_task!"), ShouldBeTrue)

							testTask, err = model.FindTask(testTask.Id)
							testutil.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)

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

	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskFailures")

	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a failing test and live API server over "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, "failing_task",
						"linux-64", NoPatch, t)
					testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := createAgent(testServer, testTask)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")

					// actually run the task.
					// this function won't return until the whole thing is done.
					testAgent.RunTask()
					time.Sleep(100 * time.Millisecond)
					testAgent.APILogger.FlushAndWait()
					printLogsForTask(testTask.Id)

					Convey("the pre and post-run scripts should have run", func() {
						So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)

						Convey("the task should have run up until its first failure", func() {
							So(scanLogsForTask(testTask.Id, "starting failing_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with failing_task!"), ShouldBeFalse)
						})

						Convey("the tasks's final status should be FAILED", func() {
							testTask, err = model.FindTask(testTask.Id)
							testutil.HandleTestingErr(err, t, "Failed to find test task")
							So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
							So(testTask.Details.Status, ShouldEqual, evergreen.TaskFailed)
							So(testTask.Details.Description, ShouldEqual, "failing shell command")
							So(testTask.Details.TimedOut, ShouldBeFalse)
							So(testTask.Details.Type, ShouldEqual, model.SystemCommandType)
						})
					})
				})
			})
		}
	}
}

func TestTaskAbortion(t *testing.T) {
	setupTlsConfigs(t)

	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskAbortion")
	for tlsString, tlsConfig := range tlsConfigs {
		for _, testSetup := range testSetups {
			Convey(testSetup.testSpec, t, func() {
				Convey("With agent running a slow test and live API server over "+tlsString, func() {
					testTask, _, err := setupAPITestData(testConfig, "very_slow_task", "linux-64", NoPatch, t)
					testutil.HandleTestingErr(err, t, "Failed to find test task")
					testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					testAgent, err := createAgent(testServer, testTask)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")

					Convey("when the abort signal is triggered on the task", func() {
						go func() {
							// Wait for a few seconds, then switch the task to aborted!
							time.Sleep(3 * time.Second)
							err := testTask.Abort("", true)
							testutil.HandleTestingErr(err, t, "Failed to abort test task")
							fmt.Println("aborted task.")
						}()

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err := testAgent.RunTask()
						So(err, ShouldBeNil)

						testAgent.APILogger.Flush()
						time.Sleep(1 * time.Second)
						printLogsForTask(testTask.Id)

						Convey("the pre and post-run scripts should have run", func() {
							So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "Received abort signal - stopping."), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "done with very_slow_task!"), ShouldBeFalse)
							testTask, err = model.FindTask(testTask.Id)
							testutil.HandleTestingErr(err, t, "Failed to find test task")
							So(testTask.Status, ShouldEqual, evergreen.TaskUndispatched)
						})
					})
				})
			})
		}
	}
}

func TestTaskTimeout(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "timeout_task", "linux-64", NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Expansions["api_httpscert"])
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				testAgent.RunTask()
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = model.FindTask(testTask.Id)
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")
				})
			})
		})
	}
}

func TestTaskCallbackTimeout(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With an agent with callback_timeout_secs=1 and a live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "timeout_task", "linux-64", NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Expansions["api_httpscert"])
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)
			prependConfigToVersion(t, testTask.Version, "callback_timeout_secs: 1\n")

			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				testAgent.RunTask()
				testAgent.APILogger.Flush()
				time.Sleep(7 * time.Second)
				//printLogsForTask(testTask.Id)
				So(testAgent.taskConfig.Project.CallbackTimeout, ShouldEqual, 1)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = model.FindTask(testTask.Id)
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")

					Convey("and the timeout task should have failed", func() {
						So(scanLogsForTask(testTask.Id, "running task-timeout commands in 1"), ShouldBeTrue)
					})
				})
			})
		})
	}
}

func TestTaskExecTimeout(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, err := setupAPITestData(testConfig, "exec_timeout_task", "linux-64", NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := New(testServer.URL, testTask.Id, testTask.Secret, "", testConfig.Expansions["api_httpscert"])
			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				testAgent.RunTask()
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = model.FindTask(testTask.Id)
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")
				})
			})
		})
	}
}

func TestTaskEndEndpoint(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		testTask, _, err := setupAPITestData(testConfig, "random", "linux-64", NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := apiserver.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins, Verbose)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			testAgent, err := createAgent(testServer, testTask)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			testAgent.heartbeater.Interval = 10 * time.Second
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling end() should update task's/host's status properly "+
				"and start running the next task", func() {
				subsequentTaskId := testTask.Id + "Two"
				details := &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}
				taskEndResp, err := testAgent.End(details)
				time.Sleep(1 * time.Second)
				So(err, ShouldBeNil)

				taskUpdate, err := model.FindTask(testTask.Id)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, evergreen.TaskSucceeded)

				testHost, err := host.FindOne(host.ById(testTask.HostId))
				So(err, ShouldBeNil)
				So(testHost.RunningTask, ShouldEqual, subsequentTaskId)

				taskUpdate, err = model.FindTask(subsequentTaskId)
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, evergreen.TaskDispatched)

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

func setupAPITestData(testConfig *evergreen.Settings, taskDisplayName string,
	variant string, patchMode patchTestMode, t *testing.T) (*model.Task, *build.Build, error) {
	//ignore errs here because the ns might just not exist.
	clearDataMsg := "Failed to clear test data collection"
	testCollections := []string{
		model.TasksCollection, build.Collection, host.Collection,
		distro.Collection, version.Collection, patch.Collection,
		model.PushlogCollection, model.ProjectVarsCollection, model.TaskQueuesCollection,
		manifest.Collection, model.ProjectRefCollection}
	testutil.HandleTestingErr(dbutil.ClearCollections(testCollections...), t, clearDataMsg)
	projectVars := &model.ProjectVars{
		Id: "evergreen-ci-render",
		Vars: map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"fetch_key":  "fetch_expansion_value",
		},
	}
	_, err := projectVars.Upsert()
	testutil.HandleTestingErr(err, t, clearDataMsg)

	taskOne := &model.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      "evergreen-ci-render",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       evergreen.TaskDispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}

	taskTwo := &model.Task{
		Id:           "testTaskIdTwo",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      "evergreen-ci-render",
		DisplayName:  taskDisplayName,
		HostId:       "",
		Secret:       "testTaskSecret",
		Activated:    true,
		Version:      "testVersionId",
		Status:       evergreen.TaskUndispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}

	if patchMode != NoPatch {
		taskOne.Requester = evergreen.PatchVersionRequester
	}

	testutil.HandleTestingErr(taskOne.Insert(), t, "failed to insert taskOne")
	testutil.HandleTestingErr(taskTwo.Insert(), t, "failed to insert taskTwo")

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
	testutil.HandleTestingErr(taskQueue.Save(), t, "failed to insert taskqueue")
	workDir, err := ioutil.TempDir("", "agent_test_")
	testutil.HandleTestingErr(err, t, "failed to create working directory")

	host := &host.Host{
		Id:   "testHost",
		Host: "testHost",
		Distro: distro.Distro{
			Id:         "test-distro-one",
			WorkDir:    workDir,
			Expansions: []distro.Expansion{{"distro_exp", "DISTRO_EXP"}},
		},
		RunningTask:   "testTaskId",
		StartedBy:     evergreen.User,
		AgentRevision: agentRevision,
	}
	testutil.HandleTestingErr(host.Insert(), t, "failed to insert host")

	// read in the project configuration
	projectFile := "testdata/config_test_plugin/project/evergreen-ci-render.yml"
	projectConfig, err := ioutil.ReadFile(projectFile)
	testutil.HandleTestingErr(err, t, "failed to read project config")

	projectRef := &model.ProjectRef{
		Identifier: "evergreen-ci-render",
		Owner:      "evergreen-ci",
		Repo:       "render",
		RepoKind:   "github",
		Branch:     "master",
		Enabled:    true,
		BatchTime:  180,
	}

	testutil.HandleTestingErr(projectRef.Insert(), t, "failed to insert projectRef")

	err = testutil.CreateTestLocalConfig(testConfig, "evergreen-ci-render", "testdata/config_test_plugin/project/evergreen-ci-render.yml")
	testutil.HandleTestingErr(err, t, "failed to marshall project config")

	// unmarshall the project configuration into a struct
	project := &model.Project{}
	testutil.HandleTestingErr(yaml.Unmarshal(projectConfig, project), t, "failed to unmarshal project config")

	// now then marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(project)
	testutil.HandleTestingErr(err, t, "failed to marshall project config")

	// insert the version document
	v := &version.Version{
		Id:       "testVersionId",
		BuildIds: []string{taskOne.BuildId},
		Config:   string(projectYamlBytes),
	}

	testutil.HandleTestingErr(v.Insert(), t, "failed to insert version")
	if patchMode != NoPatch {
		mainPatchContent, err := ioutil.ReadFile("testdata/test.patch")
		testutil.HandleTestingErr(err, t, "failed to read test patch file")
		modulePatchContent, err := ioutil.ReadFile("testdata/testmodule.patch")
		testutil.HandleTestingErr(err, t, "failed to read test module patch file")

		ptch := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: v.Id,
		}
		if patchMode == InlinePatch {
			ptch.Patches = []patch.ModulePatch{
				{
					ModuleName: "",
					Githash:    "1e5232709595db427893826ce19289461cba3f75",
					PatchSet:   patch.PatchSet{Patch: string(mainPatchContent)},
				},
				{
					ModuleName: "recursive",
					Githash:    "1e5232709595db427893826ce19289461cba3f75",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			}
		} else {
			p1Id, p2Id := bson.NewObjectId().Hex(), bson.NewObjectId().Hex()
			So(dbutil.WriteGridFile(patch.GridFSPrefix, p1Id, strings.NewReader(string(mainPatchContent))), ShouldBeNil)
			So(dbutil.WriteGridFile(patch.GridFSPrefix, p2Id, strings.NewReader(string(modulePatchContent))), ShouldBeNil)
			ptch.Patches = []patch.ModulePatch{
				{
					ModuleName: "",
					Githash:    "1e5232709595db427893826ce19289461cba3f75",
					PatchSet:   patch.PatchSet{PatchFileId: p1Id},
				},
				{
					ModuleName: "recursive",
					Githash:    "1e5232709595db427893826ce19289461cba3f75",
					PatchSet:   patch.PatchSet{PatchFileId: p2Id},
				},
			}
		}
		testutil.HandleTestingErr(ptch.Insert(), t, "failed to insert patch")
	}

	session, _, err := dbutil.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(err, t, "couldn't get db session!")

	// Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).
		RemoveAll(bson.M{"t_id": bson.M{"$in": []string{taskOne.Id, taskTwo.Id}}})
	testutil.HandleTestingErr(err, t, "failed to remove logs")

	build := &build.Build{
		Id: "testBuildId",
		Tasks: []build.TaskCache{
			build.NewTaskCache(taskOne.Id, taskOne.DisplayName, true),
			build.NewTaskCache(taskTwo.Id, taskTwo.DisplayName, true),
		},
		Version: "testVersionId",
	}

	testutil.HandleTestingErr(build.Insert(), t, "failed to insert build")
	return taskOne, build, nil
}

// prependConfigToVersion modifies the project config with the given id
func prependConfigToVersion(t *testing.T, versionId, configData string) {
	v, err := version.FindOne(version.ById(versionId))
	testutil.HandleTestingErr(err, t, "failed to load version")
	if v == nil {
		err = fmt.Errorf("could not find version to update")
		testutil.HandleTestingErr(err, t, "failed to find version")
	}
	v.Config = configData + v.Config
	testutil.HandleTestingErr(dbutil.ClearCollections(version.Collection), t, "couldnt reset version")
	testutil.HandleTestingErr(v.Insert(), t, "failed to insert version")
}
