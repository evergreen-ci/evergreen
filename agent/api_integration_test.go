package agent

import (
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/command"
	dbutil "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

var testConfig = testutil.TestConfig()

var testSetups []testConfigPath

var buildVariantsToTest = []string{"linux-64", "windows8"}

var tlsConfigs map[string]*tls.Config

// the revision we'll assume is the current one for the agent. this is the
// same as appears in testdata/executables/version
var agentRevision = "xxx"

var testDirectory string

// NoopSignal is a signal handler that ignores all signals, so that we can
// intercept and check for them directly in the test instead
type NoopSignalHandler struct{}

type testConfigPath struct {
	testSpec string
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
	testSetups = []testConfigPath{
		{"With plugin mode test config"},
	}
	reporting.QuietMode()
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
			tls.X509KeyPair([]byte(testConfig.Api.HttpsCert),
				[]byte(testConfig.Api.HttpsKey))
		if err != nil {
			testutil.HandleTestingErr(err, t, "X509KeyPair failed during test initialization: %v", err)
		}
		tlsConfigs = map[string]*tls.Config{
			"http": nil,
			// TODO: do tests over SSL, see EVG-1512
			// "https": tlsConfig,
		}
	}
}

func createAgent(testServer *service.TestServer, testHost *host.Host) (*Agent, error) {
	testAgent, err := New(Options{
		APIURL:      testServer.URL,
		HostId:      testHost.Id,
		HostSecret:  testHost.Secret,
		Certificate: testConfig.Api.HttpsCert,
	})
	if err != nil {
		return nil, err
	}
	testAgent.heartbeater.Interval = 10 * time.Second
	testAgent.taskConfig = &model.TaskConfig{Expansions: &command.Expansions{}}
	return testAgent, nil
}

func assignAgentTask(agt *Agent, testTask *task.Task) error {
	// modify task communicator
	httpTaskComm, ok := agt.TaskCommunicator.(*comm.HTTPCommunicator)
	if !ok {
		message := "error getting http communicator for agent"
		return errors.New(message)
	}

	httpTaskComm.TaskId = testTask.Id
	httpTaskComm.TaskSecret = testTask.Secret

	agt.TaskCommunicator = httpTaskComm
	return nil
}

func TestBasicEndpoints(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		testTask, _, h, err := setupAPITestData(testConfig, "task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			defer testAgent.stop()
			So(assignAgentTask(testAgent, testTask), ShouldBeNil)

			Convey("sending logs should store the log messages properly", func() {
				msg1 := "task logger initialized!"
				msg2 := "system logger initialized!"
				msg3 := "exec logger initialized!"
				testAgent.logger.LogTask(slogger.INFO, msg1)
				testAgent.logger.LogSystem(slogger.INFO, msg2)
				testAgent.logger.LogExecution(slogger.INFO, msg3)
				time.Sleep(1 * time.Second)
				testAgent.APILogger.FlushAndWait()

				// This returns logs in order of NEWEST first.
				logMessages, err := model.FindMostRecentLogMessages(testTask.Id, 0, 10, []string{}, []string{})
				testutil.HandleTestingErr(err, t, "failed to get log msgs")
				So(logMessages[2].Message, ShouldEndWith, msg1)
				So(logMessages[1].Message, ShouldEndWith, msg2)
				So(logMessages[0].Message, ShouldEndWith, msg3)
				Convey("Task endpoints should work between agent and server", func() {
					testAgent.StartBackgroundActions(&NoopSignalHandler{})
					Convey("calling GetTask should get retrieve same task back", func() {
						var testTaskFromApi *task.Task
						testTaskFromApi, err = testAgent.GetTask()
						So(err, ShouldBeNil)

						// ShouldResemble doesn't seem to work here, possibly because of
						// omitempty? anyways, just assert equality of the important fields
						So(testTaskFromApi.Id, ShouldEqual, testTask.Id)
						So(testTaskFromApi.Status, ShouldEqual, testTask.Status)
						So(testTaskFromApi.HostId, ShouldEqual, testTask.HostId)
					})

					Convey("calling start should flip the task's status to started", func() {
						var testHost *host.Host
						err = testAgent.Start()
						testutil.HandleTestingErr(err, t, "Couldn't start task: %v", err)
						testTask, err = task.FindOne(task.ById(testTask.Id))
						testutil.HandleTestingErr(err, t, "Couldn't refresh task from db: %v", err)
						So(testTask.Status, ShouldEqual, evergreen.TaskStarted)
						testHost, err = host.FindOne(host.ByRunningTaskId(testTask.Id))
						So(err, ShouldBeNil)
						So(testHost.Id, ShouldEqual, "testHost")
						So(testHost.RunningTask, ShouldEqual, testTask.Id)
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
						_, err = testAgent.End(details)
						So(err, ShouldBeNil)
						time.Sleep(100 * time.Millisecond)
						taskUpdate, err := task.FindOne(task.ById(testTask.Id))
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
						So(timeoutSignal, ShouldEqual, comm.IdleTimeout)
					})
				})
			})
		})
	}
}

func TestHeartbeatSignals(t *testing.T) {
	setupTlsConfigs(t)

	for tlsString, tlsConfig := range tlsConfigs {
		_, _, h, err := setupAPITestData(testConfig, evergreen.CompileStage, "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			defer testAgent.stop()

			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				So(testServer.Listener.Close(), ShouldBeNil)
				signal, ok := <-testAgent.signalHandler.heartbeatChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, comm.HeartbeatMaxFailed)
			})
		})
	}
}

func TestAgentDirectorySuccess(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent printing directory and live API server over "+tlsString, t, func() {
			testTask, _, testHost, err := setupAPITestData(testConfig, "print_dir_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, testHost)
			testutil.HandleTestingErr(err, t, "Failed to start agent")
			defer testAgent.stop()

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)

			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			dir, err := os.Getwd()

			testutil.HandleTestingErr(err, t, "Failed to read current directory")

			_, err = testAgent.RunTask()
			So(err, ShouldBeNil)
			printLogsForTask(testTask.Id)
			distro, err := testAgent.GetDistro()
			testutil.HandleTestingErr(err, t, "Failed to get agent distro")

			h := md5.New()
			_, err = h.Write([]byte(fmt.Sprintf("%s_%d_%d", testTask.Id, 0, os.Getpid())))
			So(err, ShouldBeNil)
			dirName := hex.EncodeToString(h.Sum(nil))
			newDir := filepath.Join(distro.WorkDir, dirName)

			Convey("Then the directory should have been set and printed", func() {
				So(scanLogsForTask(testTask.Id, "", "printing current directory"), ShouldBeTrue)
				So(scanLogsForTask(testTask.Id, "", newDir), ShouldBeTrue)
			})
			Convey("Then the directory should have been deleted", func() {
				var files []os.FileInfo
				files, err = ioutil.ReadDir("./")
				testutil.HandleTestingErr(err, t, "Failed to read current directory")
				for _, f := range files {
					So(f.Name(), ShouldNotEqual, newDir)
				}
			})
			err = os.Chdir(dir)
			testutil.HandleTestingErr(err, t, "Failed to change directory back to main dir")
		})
	}
}
func TestAgentDirectoryFailure(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent printing directory and live API server over "+tlsString, t, func() {
			testTask, _, testHost, err := setupAPITestData(testConfig, "print_dir_task", "linux-64",
				filepath.Join(testDirectory, "testdata", "config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, testHost)
			testutil.HandleTestingErr(err, t, "Failed to start test agent")

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)
			dir, err := os.Getwd()

			testutil.HandleTestingErr(err, t, "Failed to read current directory")

			distro, err := testAgent.GetDistro()
			testutil.HandleTestingErr(err, t, "Failed to get agent distro")

			h := md5.New()
			_, err = h.Write([]byte(
				fmt.Sprintf("%s_%d_%d", testTask.Id, 0, os.Getpid())))
			So(err, ShouldBeNil)
			dirName := hex.EncodeToString(h.Sum(nil))
			newDir := filepath.Join(distro.WorkDir, dirName)

			newDirFile, err := os.Create(newDir)
			testutil.HandleTestingErr(err, t, "Couldn't create file: %v", err)

			_, err = testAgent.RunTask()
			Convey("Then the agent should have errored", func() {
				So(err, ShouldNotBeNil)
			})

			printLogsForTask(testTask.Id)
			Convey("Then the task should not have been run", func() {
				So(scanLogsForTask(testTask.Id, "", "printing current directory"), ShouldBeFalse)
				So(scanLogsForTask(testTask.Id, "", newDir), ShouldBeFalse)
			})
			<-testAgent.KillChan
			Convey("Then the taskDetail type should have been set to SystemCommandType and have status failed", func() {
				select {
				case detail := <-testAgent.endChan:
					So(detail.Type, ShouldEqual, model.SystemCommandType)
					So(detail.Status, ShouldEqual, evergreen.TaskFailed)
				default:
					t.Errorf("unable to read from the endChan")
				}
			})
			err = os.Chdir(dir)
			testutil.HandleTestingErr(err, t, "Failed to change directory back to main dir")

			testutil.HandleTestingErr(newDirFile.Close(), t, "failed to close dummy directory, file")
			err = os.Remove(newDir)
			testutil.HandleTestingErr(err, t, "Failed to remove dummy directory file")
		})
	}
}

func TestSecrets(t *testing.T) {
	setupTlsConfigs(t)
	testTask, _, testHost, err := setupAPITestData(testConfig, evergreen.CompileStage, "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
	testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, testHost)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			defer testAgent.stop()

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)

			testAgent.heartbeater.Interval = 100 * time.Millisecond
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("killing the server should result in failure signal", func() {
				So(testServer.Listener.Close(), ShouldBeNil)
				signal, ok := <-testAgent.signalHandler.heartbeatChan
				So(ok, ShouldBeTrue)
				So(signal, ShouldEqual, comm.HeartbeatMaxFailed)
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
						testTask, _, h, err := setupAPITestData(testConfig, "compile", variant, filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
						testutil.HandleTestingErr(err, t, "Couldn't create test task: %v", err)
						testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						defer testServer.Close()
						testAgent, err := createAgent(testServer, h)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")
						defer testAgent.stop()
						So(assignAgentTask(testAgent, testTask), ShouldBeNil)

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err = testAgent.RunTask()
						So(err, ShouldBeNil)
						Convey("expansions should be fetched", func() {
							So(testAgent.taskConfig.Expansions.Get("aws_key"), ShouldEqual, testConfig.Providers.AWS.Id)
							So(scanLogsForTask(testTask.Id, "", "fetch_expansion_value"), ShouldBeTrue)
						})
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()
						printLogsForTask(testTask.Id)

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "", "Executing script with sh: echo \"predefined command!\""), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "predefined command!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "this should not end up in the logs"), ShouldBeFalse)
							So(scanLogsForTask(testTask.Id, "", "Command timeout set to 21m40s"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "Command timeout set to 43m20s"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "Cloning into") || // git 1.8
								scanLogsForTask(testTask.Id, "", "Initialized empty Git repository"), // git 1.7
								ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "i am compiling!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "i am sanity testing!"), ShouldBeTrue)

							// Check that functions with args are working correctly
							So(scanLogsForTask(testTask.Id, "", "arg1 is FOO"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "arg2 is BAR"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "arg3 is Expanded: qux"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "arg4 is Default: default_value"), ShouldBeTrue)

							// Check that multi-command functions are working correctly
							So(scanLogsForTask(testTask.Id, "", "step 1 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "step 2 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "step 3 of multi-command func"), ShouldBeTrue)

							testTask, err = task.FindOne(task.ById(testTask.Id))
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
						testTask, _, h, err := setupAPITestData(testConfig, "normal_task", variant, filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
						testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
						testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						defer testServer.Close()
						testAgent, err := createAgent(testServer, h)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")
						defer testAgent.stop()
						So(assignAgentTask(testAgent, testTask), ShouldBeNil)

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err = testAgent.RunTask()
						So(err, ShouldBeNil)
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)

							So(scanLogsForTask(testTask.Id, "", "starting normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "done with normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, model.SystemLogPrefix, "this output should go to the system logs."), ShouldBeTrue)

							testTask, err = task.FindOne(task.ById(testTask.Id))
							testutil.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)

							expectedResults := []task.TestResult{
								{
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
					testTask, _, h, err := setupAPITestData(testConfig, "failing_task",
						"linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
					testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
					testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					defer testServer.Close()
					testAgent, err := createAgent(testServer, h)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")
					defer testAgent.stop()
					So(assignAgentTask(testAgent, testTask), ShouldBeNil)

					// actually run the task.
					// this function won't return until the whole thing is done.
					_, err = testAgent.RunTask()
					So(err, ShouldBeNil)
					time.Sleep(100 * time.Millisecond)
					testAgent.APILogger.FlushAndWait()
					printLogsForTask(testTask.Id)

					Convey("the pre and post-run scripts should have run", func() {
						So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
						So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)

						Convey("the task should have run up until its first failure", func() {
							So(scanLogsForTask(testTask.Id, "", "starting failing_task!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "done with failing_task!"), ShouldBeFalse)
						})

						Convey("the tasks's final status should be FAILED", func() {
							testTask, err = task.FindOne(task.ById(testTask.Id))
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
					testTask, _, h, err := setupAPITestData(testConfig, "very_slow_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
					testutil.HandleTestingErr(err, t, "Failed to find test task")
					testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					defer testServer.Close()
					testAgent, err := createAgent(testServer, h)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")
					So(assignAgentTask(testAgent, testTask), ShouldBeNil)
					Convey("when the abort signal is triggered on the task", func() {
						go func() {
							// Wait for a few seconds, then switch the task to aborted!
							time.Sleep(3 * time.Second)
							err := model.AbortTask(testTask.Id, "")
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
							So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "Received abort signal - stopping."), ShouldBeTrue)
							So(scanLogsForTask(testTask.Id, "", "done with very_slow_task!"), ShouldBeFalse)
							testTask, err = task.FindOne(task.ById(testTask.Id))
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
			testTask, _, h, err := setupAPITestData(testConfig, "timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)
			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				//printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = task.FindOne(task.ById(testTask.Id))
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")
				})
			})
		})
	}
}

func TestFunctionVariantExclusion(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		// test against the windows8 and linux-64 variants; linux-64 excludes a test command
		for _, variant := range []string{"windows8", "linux-64"} {
			Convey("With agent running a "+variant+" task and live API server over "+tlsString, t, func() {
				testTask, _, h, err := setupAPITestData(testConfig, "variant_test", variant, filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
				testutil.HandleTestingErr(err, t, "Failed to find test task")
				testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
				testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
				defer testServer.Close()
				testAgent, err := createAgent(testServer, h)
				testutil.HandleTestingErr(err, t, "failed to create agent: %v")

				So(assignAgentTask(testAgent, testTask), ShouldBeNil)
				Convey("running the task", func() {
					_, err = testAgent.RunTask()
					So(err, ShouldBeNil)
					testAgent.APILogger.Flush()
					if variant == "windows8" {
						Convey("the variant-specific function command should run", func() {
							So(scanLogsForTask(testTask.Id, "", "variant not excluded!"), ShouldBeTrue)
						})
					} else {
						Convey("the variant-specific function command should not run", func() {
							So(scanLogsForTask(testTask.Id, "", "variant not excluded!"), ShouldBeFalse)
							So(scanLogsForTask(testTask.Id, "", "Skipping command 'shell.exec'"), ShouldBeTrue)
						})
					}
				})
			})
		}
	}
}

func TestTaskCallbackTimeout(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With an agent with callback_timeout_secs=1 and a live API server over "+tlsString, t, func() {
			testTask, _, h, err := setupAPITestData(testConfig, "timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)
			prependConfigToVersion(t, testTask.Version, "callback_timeout_secs: 1\n")

			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(7 * time.Second)
				//printLogsForTask(testTask.Id)
				So(testAgent.taskConfig.Project.CallbackTimeout, ShouldEqual, 1)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = task.FindOne(task.ById(testTask.Id))
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")

					Convey("and the timeout task should have failed", func() {
						So(scanLogsForTask(testTask.Id, "", "running task-timeout commands in 1"), ShouldBeTrue)
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
			testTask, _, h, err := setupAPITestData(testConfig, "exec_timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)
			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = task.FindOne(task.ById(testTask.Id))
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")
				})
			})
		})
	}
}

func TestProjectTaskExecTimeout(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With agent running a slow test and live API server over "+tlsString, t, func() {
			testTask, _, h, err := setupAPITestData(testConfig, "project_exec_timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/project-timeout-test.yml"), NoPatch, t)
			testutil.HandleTestingErr(err, t, "Failed to find test task")

			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			So(assignAgentTask(testAgent, testTask), ShouldBeNil)

			Convey("after the slow test runs beyond the project timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(testTask.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(testTask.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(testTask.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err = task.FindOne(task.ById(testTask.Id))
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
		testTask, _, h, err := setupAPITestData(testConfig, "random", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), NoPatch, t)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, h)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			So(assignAgentTask(testAgent, testTask), ShouldBeNil)

			testAgent.heartbeater.Interval = 10 * time.Second
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling end() should update task's/host's status properly "+
				"and start running the next task", func() {
				details := &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}
				taskEndResp, err := testAgent.End(details)
				time.Sleep(1 * time.Second)
				So(err, ShouldBeNil)
				So(taskEndResp.ShouldExit, ShouldBeFalse)

				taskUpdate, err := task.FindOne(task.ById(testTask.Id))
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, evergreen.TaskSucceeded)

				testHost, err := host.FindOne(host.ById(testTask.HostId))
				So(err, ShouldBeNil)
				So(testHost.RunningTask, ShouldEqual, "")

			})
		})
	}
}

// scanLogsForTask examines log messages stored under the given taskId and returns true
// if the string scanFor appears in their contents at least once. If a non-empty value is supplied
// for logTypes, only logs within the specified log types will
// be scanned for the given string (see SystemLogPrefix, AgentLogPrefix, TaskLogPrefix).
func scanLogsForTask(taskId string, logTypes string, scanFor string) bool {
	taskLogs, err := model.FindAllTaskLogs(taskId, 0)
	if err != nil {
		panic(err)
	}
	for _, taskLogObj := range taskLogs {
		for _, logmsg := range taskLogObj.Messages {
			if logTypes != "" && !strings.Contains(logTypes, logmsg.Type) {
				continue
			}
			if strings.Contains(strings.ToLower(logmsg.Message), strings.ToLower(scanFor)) {
				return true
			}
		}
	}
	return false
}

func printLogsForTask(taskId string) {
	logMessages, err := model.FindMostRecentLogMessages(taskId, 0, 100, []string{}, []string{})
	if err != nil {
		panic(err)
	}
	for i := len(logMessages) - 1; i >= 0; i-- {
		if logMessages[i].Type == model.SystemLogPrefix {
			continue
		}
		fmt.Println(logMessages[i].Message)
	}
}

func setupAPITestData(testConfig *evergreen.Settings, taskDisplayName string,
	variant string, projectFile string, patchMode patchTestMode, t *testing.T) (*task.Task, *build.Build, *host.Host, error) {
	// Ignore errs here because the ns might just not exist.
	clearDataMsg := "Failed to clear test data collection"
	testCollections := []string{
		task.Collection, build.Collection, host.Collection,
		distro.Collection, version.Collection, patch.Collection,
		model.PushlogCollection, model.ProjectVarsCollection, model.TaskQueuesCollection,
		manifest.Collection, model.ProjectRefCollection}
	testutil.HandleTestingErr(dbutil.ClearCollections(testCollections...), t, clearDataMsg)

	// Read in the project configuration
	projectConfig, err := ioutil.ReadFile(projectFile)
	testutil.HandleTestingErr(err, t, "failed to read project config")

	// Unmarshall the project configuration into a struct
	project := &model.Project{}
	testutil.HandleTestingErr(model.LoadProjectInto(projectConfig, "test", project),
		t, "failed to unmarshal project config")

	// Marshall the project YAML for storage
	projectYamlBytes, err := yaml.Marshal(project)
	testutil.HandleTestingErr(err, t, "failed to marshall project config")

	// Create the ref for the project
	projectRef := &model.ProjectRef{
		Identifier:  project.DisplayName,
		Owner:       project.Owner,
		Repo:        project.Repo,
		RepoKind:    project.RepoKind,
		Branch:      project.Branch,
		Enabled:     project.Enabled,
		BatchTime:   project.BatchTime,
		LocalConfig: string(projectConfig),
	}
	testutil.HandleTestingErr(projectRef.Insert(), t, "failed to insert projectRef")

	// Save the project variables
	projectVars := &model.ProjectVars{
		Id: project.DisplayName,
		Vars: map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"fetch_key":  "fetch_expansion_value",
		},
	}
	_, err = projectVars.Upsert()
	testutil.HandleTestingErr(err, t, clearDataMsg)

	hostId := "testHost"
	// Create and insert two tasks
	taskOne := &task.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      project.DisplayName,
		DisplayName:  taskDisplayName,
		HostId:       hostId,
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       evergreen.TaskDispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	if patchMode != NoPatch {
		taskOne.Requester = evergreen.PatchVersionRequester
	}
	testutil.HandleTestingErr(taskOne.Insert(), t, "failed to insert taskOne")

	taskTwo := &task.Task{
		Id:           "testTaskIdTwo",
		BuildId:      "testBuildId",
		DistroId:     "test-distro-one",
		BuildVariant: variant,
		Project:      project.DisplayName,
		DisplayName:  taskDisplayName,
		HostId:       "",
		Secret:       "testTaskSecret",
		Version:      "testVersionId",
		Status:       evergreen.TaskUndispatched,
		Requester:    evergreen.RepotrackerVersionRequester,
		Activated:    true,
	}
	testutil.HandleTestingErr(taskTwo.Insert(), t, "failed to insert taskTwo")

	// Set up a task queue for task end tests
	taskQueue := &model.TaskQueue{
		Distro: "test-distro-one",
		Queue: []model.TaskQueueItem{
			{
				Id:          "testTaskIdTwo",
				DisplayName: taskDisplayName,
			},
		},
	}
	testutil.HandleTestingErr(taskQueue.Save(), t, "failed to insert taskqueue")

	// Insert the version document
	v := &version.Version{
		Id:       "testVersionId",
		BuildIds: []string{taskOne.BuildId},
		Config:   string(projectYamlBytes),
	}
	testutil.HandleTestingErr(v.Insert(), t, "failed to insert version")

	// Insert the build that contains the tasks
	build := &build.Build{
		Id: "testBuildId",
		Tasks: []build.TaskCache{
			build.NewTaskCache(taskOne.Id, taskOne.DisplayName, true),
			build.NewTaskCache(taskTwo.Id, taskTwo.DisplayName, true),
		},
		Version: v.Id,
	}
	testutil.HandleTestingErr(build.Insert(), t, "failed to insert build")

	workDir, err := ioutil.TempDir("", "agent_test_")
	testutil.HandleTestingErr(err, t, "failed to create working directory")

	// Insert the host info for running the tests
	host := &host.Host{
		Id:   "testHost",
		Host: "testHost",
		Distro: distro.Distro{
			Id:         "test-distro-one",
			WorkDir:    workDir,
			Expansions: []distro.Expansion{{"distro_exp", "DISTRO_EXP"}},
		},
		Provider:      evergreen.HostTypeStatic,
		RunningTask:   taskOne.Id,
		Secret:        "testHostSecret",
		StartedBy:     evergreen.User,
		AgentRevision: agentRevision,
	}
	testutil.HandleTestingErr(host.Insert(), t, "failed to insert host")

	session, _, err := dbutil.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(err, t, "couldn't get db session!")

	// Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).
		RemoveAll(bson.M{"t_id": bson.M{"$in": []string{taskOne.Id, taskTwo.Id}}})
	testutil.HandleTestingErr(err, t, "failed to remove logs")

	return taskOne, build, host, nil
}

type patchRequest struct {
	moduleName string
	filePath   string
	githash    string
}

func setupPatches(patchMode patchTestMode, b *build.Build, t *testing.T, patches ...patchRequest) {
	if patchMode == NoPatch {
		return
	}

	ptch := &patch.Patch{
		Status:  evergreen.PatchCreated,
		Version: b.Version,
		Patches: []patch.ModulePatch{},
	}

	for _, p := range patches {
		patchContent, err := ioutil.ReadFile(p.filePath)
		testutil.HandleTestingErr(err, t, "failed to read test patch file")

		if patchMode == InlinePatch {
			ptch.Patches = append(ptch.Patches, patch.ModulePatch{
				ModuleName: p.moduleName,
				Githash:    p.githash,
				PatchSet:   patch.PatchSet{Patch: string(patchContent)},
			})
		} else {
			pId := bson.NewObjectId().Hex()
			So(dbutil.WriteGridFile(patch.GridFSPrefix, pId, strings.NewReader(string(patchContent))), ShouldBeNil)
			ptch.Patches = append(ptch.Patches, patch.ModulePatch{
				ModuleName: p.moduleName,
				Githash:    p.githash,
				PatchSet:   patch.PatchSet{PatchFileId: pId},
			})
		}
	}
	testutil.HandleTestingErr(ptch.Insert(), t, "failed to insert patch")
}

// prependConfigToVersion modifies the project config with the given id
func prependConfigToVersion(t *testing.T, versionId, configData string) {
	v, err := version.FindOne(version.ById(versionId))
	testutil.HandleTestingErr(err, t, "failed to load version")
	if v == nil {
		err = errors.New("could not find version to update")
		testutil.HandleTestingErr(err, t, "failed to find version")
	}
	v.Config = configData + v.Config
	testutil.HandleTestingErr(dbutil.ClearCollections(version.Collection), t, "couldnt reset version")
	testutil.HandleTestingErr(v.Insert(), t, "failed to insert version")
}

func TestMain(m *testing.M) {
	var err error

	testDirectory = testutil.GetDirectoryOfFile()
	exitCode := m.Run()

	err = os.Chdir(testDirectory)
	if err != nil {
		panic(err)
	}

	os.Exit(exitCode)
}
