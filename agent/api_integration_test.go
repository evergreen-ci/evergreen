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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
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
)

var testConfig = testutil.TestConfig()

var testSetups []testConfigPath

var buildVariantsToTest = []string{"linux-64", "windows8"}

var tlsConfigs map[string]*tls.Config

var testDirectory string

// NoopSignal is a signal handler that ignores all signals, so that we can
// intercept and check for them directly in the test instead
type NoopSignalHandler struct{}

type testConfigPath struct {
	testSpec string
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

func assignAgentTask(agt *Agent, testTask *task.Task) {
	agt.SetTask(testTask.Id, testTask.Secret)
}

func TestBasicEndpoints(t *testing.T) {
	setupTlsConfigs(t)
	for tlsString, tlsConfig := range tlsConfigs {
		modelData, err := modelutil.SetupAPITestData(testConfig, "task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			defer testAgent.stop()

			assignAgentTask(testAgent, modelData.Task)

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
				logMessages, err := model.FindMostRecentLogMessages(modelData.Task.Id, 0, 10, []string{}, []string{})
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
						So(testTaskFromApi.Id, ShouldEqual, modelData.Task.Id)
						So(testTaskFromApi.Status, ShouldEqual, modelData.Task.Status)
						So(testTaskFromApi.HostId, ShouldEqual, modelData.Task.HostId)
					})

					Convey("calling start should flip the task's status to started", func() {
						var testHost *host.Host
						var testTask *task.Task
						err = testAgent.Start()
						testutil.HandleTestingErr(err, t, "Couldn't start task: %v", err)

						testTask, err = task.FindOne(task.ById(modelData.Task.Id))
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
						taskUpdate, err := task.FindOne(task.ById(modelData.Task.Id))
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
		modelData, err := modelutil.SetupAPITestData(testConfig, evergreen.CompileStage, "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
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

			modelData, err := modelutil.SetupAPITestData(testConfig, "print_dir_task", "linux-64", filepath.Join(testDirectory,
				"testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test data")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "Failed to start agent")
			defer testAgent.stop()

			assignAgentTask(testAgent, modelData.Task)

			So(err, ShouldBeNil)
			So(testAgent, ShouldNotBeNil)

			dir, err := os.Getwd()

			testutil.HandleTestingErr(err, t, "Failed to read current directory")

			distro, err := testAgent.GetDistro()
			testutil.HandleTestingErr(err, t, "Failed to get agent distro")
			_, err = testAgent.RunTask()
			So(err, ShouldBeNil)

			printLogsForTask(modelData.Task.Id)

			h := md5.New()
			_, err = h.Write([]byte(fmt.Sprintf("%s_%d_%d", modelData.Task.Id, 0, os.Getpid())))
			So(err, ShouldBeNil)

			dirName := hex.EncodeToString(h.Sum(nil))
			newDir := filepath.Join(distro.WorkDir, dirName)

			Convey("Then the directory should have been set and printed", func() {
				So(scanLogsForTask(modelData.Task.Id, "", "printing current directory"), ShouldBeTrue)
				So(scanLogsForTask(modelData.Task.Id, "", newDir), ShouldBeTrue)
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
			modelData, err := modelutil.SetupAPITestData(testConfig, "print_dir_task", "linux-64",
				filepath.Join(testDirectory, "testdata", "config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "Failed to start test agent")

			assignAgentTask(testAgent, modelData.Task)
			dir, err := os.Getwd()

			testutil.HandleTestingErr(err, t, "Failed to read current directory")

			distro, err := testAgent.GetDistro()
			testutil.HandleTestingErr(err, t, "Failed to get agent distro")

			h := md5.New()

			_, err = h.Write([]byte(fmt.Sprintf("%s_%d_%d", modelData.Task.Id, 0,
				os.Getpid())))
			So(err, ShouldBeNil)
			dirName := hex.EncodeToString(h.Sum(nil))
			newDir := filepath.Join(distro.WorkDir, dirName)

			newDirFile, err := os.Create(newDir)
			testutil.HandleTestingErr(err, t, "Couldn't create file: %v", err)

			_, err = testAgent.RunTask()
			Convey("Then the agent should have errored", func() {
				So(err, ShouldNotBeNil)
			})

			printLogsForTask(modelData.Task.Id)
			Convey("Then the task should not have been run", func() {
				So(scanLogsForTask(modelData.Task.Id, "", "printing current directory"), ShouldBeFalse)
				So(scanLogsForTask(modelData.Task.Id, "", newDir), ShouldBeFalse)
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
	modelData, err := modelutil.SetupAPITestData(testConfig, evergreen.CompileStage, "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
	testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

	for tlsString, tlsConfig := range tlsConfigs {
		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")
			defer testAgent.stop()

			assignAgentTask(testAgent, modelData.Task)

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
						modelData, err := modelutil.SetupAPITestData(testConfig, "compile", variant, filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
						testutil.HandleTestingErr(err, t, "Couldn't create test task: %v", err)
						testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						defer testServer.Close()
						testAgent, err := createAgent(testServer, modelData.Host)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")
						defer testAgent.stop()
						assignAgentTask(testAgent, modelData.Task)

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err = testAgent.RunTask()
						So(err, ShouldBeNil)
						Convey("expansions should be fetched", func() {
							So(testAgent.taskConfig.Expansions.Get("aws_key"), ShouldEqual, testConfig.Providers.AWS.Id)
							So(testAgent.taskConfig.Expansions.Get("distro_id"), ShouldEqual, testAgent.taskConfig.Distro.Id)
							So(testAgent.taskConfig.Expansions.Get("distro_id"), ShouldEqual, "test-distro-one")
							So(scanLogsForTask(modelData.Task.Id, "", "fetch_expansion_value"), ShouldBeTrue)
						})
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()
						printLogsForTask(modelData.Task.Id)

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "Executing script with sh: echo \"predefined command!\""), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "predefined command!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "this should not end up in the logs"), ShouldBeFalse)
							So(scanLogsForTask(modelData.Task.Id, "", "Command timeout set to 21m40s"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "Command timeout set to 43m20s"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "Cloning into") || // git 1.8
								scanLogsForTask(modelData.Task.Id, "", "Initialized empty Git repository"), // git 1.7
								ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "i am compiling!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "i am sanity testing!"), ShouldBeTrue)

							// Check that functions with args are working correctly
							So(scanLogsForTask(modelData.Task.Id, "", "arg1 is FOO"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "arg2 is BAR"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "arg3 is Expanded: qux"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "arg4 is Default: default_value"), ShouldBeTrue)

							// Check that multi-command functions are working correctly
							So(scanLogsForTask(modelData.Task.Id, "", "step 1 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "step 2 of multi-command func"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "step 3 of multi-command func"), ShouldBeTrue)

							testTask, err := task.FindOne(task.ById(modelData.Task.Id))
							testutil.HandleTestingErr(err, t, "Couldn't find test task: %v", err)
							So(testTask.Status, ShouldEqual, evergreen.TaskSucceeded)

							// use function display name as description when none is specified in command
							So(testTask.Details.Status, ShouldEqual, evergreen.TaskSucceeded)
							So(testTask.Details.Description, ShouldEqual, `'shell.exec' in "silent shell test"`)
							So(testTask.Details.TimedOut, ShouldBeFalse)
							So(testTask.Details.Type, ShouldEqual, model.SystemCommandType)
						})

						Convey("manifest should be created", func() {
							m, err := manifest.FindOne(manifest.ById(modelData.Task.Version))
							So(modelData.Task.Version, ShouldEqual, "testVersionId")
							So(err, ShouldBeNil)
							So(m, ShouldNotBeNil)
							So(m.ProjectName, ShouldEqual, modelData.Task.Project)
							So(m.Id, ShouldEqual, modelData.Task.Version)
							So(m.Modules, ShouldNotBeEmpty)
							So(m.Modules["recursive"], ShouldNotBeNil)
							So(m.Modules["recursive"].URL, ShouldNotEqual, "")
						})
					})

					Convey("With agent running a regular test and live API server over "+
						tlsString+" on variant "+variant, func() {
						modelData, err := modelutil.SetupAPITestData(testConfig, "normal_task", variant, filepath.Join(testDirectory,
							"testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
						testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
						testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
						testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
						defer testServer.Close()
						testAgent, err := createAgent(testServer, modelData.Host)
						testutil.HandleTestingErr(err, t, "failed to create agent: %v")
						defer testAgent.stop()
						assignAgentTask(testAgent, modelData.Task)

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err = testAgent.RunTask()
						So(err, ShouldBeNil)
						time.Sleep(100 * time.Millisecond)
						testAgent.APILogger.FlushAndWait()

						Convey("all scripts in task should have been run successfully", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)

							So(scanLogsForTask(modelData.Task.Id, "", "starting normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "done with normal_task!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, model.SystemLogPrefix, "this output should go to the system logs."), ShouldBeTrue)

							var testTask *task.Task
							testTask, err = task.FindOne(task.ById(modelData.Task.Id))
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
					modelData, err := modelutil.SetupAPITestData(testConfig, "failing_task",
						"linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
					testutil.HandleTestingErr(err, t, "Couldn't create test data: %v", err)
					testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					defer testServer.Close()
					testAgent, err := createAgent(testServer, modelData.Host)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")
					defer testAgent.stop()
					assignAgentTask(testAgent, modelData.Task)

					// actually run the task.
					// this function won't return until the whole thing is done.
					_, err = testAgent.RunTask()
					So(err, ShouldBeNil)
					time.Sleep(100 * time.Millisecond)
					testAgent.APILogger.FlushAndWait()
					printLogsForTask(modelData.Task.Id)

					Convey("the pre and post-run scripts should have run", func() {
						So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
						So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)

						Convey("the task should have run up until its first failure", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "starting failing_task!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "done with failing_task!"), ShouldBeFalse)
						})

						Convey("the tasks's final status should be FAILED", func() {
							testTask, err := task.FindOne(task.ById(modelData.Task.Id))
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
					modelData, err := modelutil.SetupAPITestData(testConfig, "very_slow_task", "linux-64", filepath.Join(testDirectory,
						"testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
					testutil.HandleTestingErr(err, t, "Failed to find test task")
					testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
					testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
					defer testServer.Close()
					testAgent, err := createAgent(testServer, modelData.Host)
					testutil.HandleTestingErr(err, t, "failed to create agent: %v")
					assignAgentTask(testAgent, modelData.Task)
					Convey("when the abort signal is triggered on the task", func() {
						go func() {
							// Wait for a few seconds, then switch the task to aborted!
							time.Sleep(2 * time.Second)
							err := model.AbortTask(modelData.Task.Id, "")
							testutil.HandleTestingErr(err, t, "Failed to abort test task")
							fmt.Println("aborted task.")
						}()

						// actually run the task.
						// this function won't return until the whole thing is done.
						_, err := testAgent.RunTask()
						So(err, ShouldBeNil)

						testAgent.APILogger.Flush()
						time.Sleep(1 * time.Second)
						printLogsForTask(modelData.Task.Id)

						Convey("the pre and post-run scripts should have run", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "Received abort signal - stopping."), ShouldBeTrue)
							So(scanLogsForTask(modelData.Task.Id, "", "done with very_slow_task!"), ShouldBeFalse)
							testTask, err := task.FindOne(task.ById(modelData.Task.Id))
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
			modelData, err := modelutil.SetupAPITestData(testConfig, "timeout_task", "linux-64",
				filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"),
				modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			assignAgentTask(testAgent, modelData.Task)
			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err := task.FindOne(task.ById(modelData.Task.Id))
					So(err, ShouldBeNil)
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
				modelData, err := modelutil.SetupAPITestData(testConfig, "variant_test", variant,
					filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
				testutil.HandleTestingErr(err, t, "Failed to find test task")

				testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
				testutil.HandleTestingErr(err, t, "Couldn't create apiserver")
				defer testServer.Close()
				testAgent, err := createAgent(testServer, modelData.Host)
				testutil.HandleTestingErr(err, t, "failed to create agent")

				assignAgentTask(testAgent, modelData.Task)
				So(modelData.Host.RunningTask, ShouldEqual, modelData.Task.Id)
				Convey("running the task", func() {

					_, err := testAgent.RunTask()
					So(err, ShouldBeNil)
					if variant == "windows8" {
						Convey("the variant-specific function command should run", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "variant not excluded!"), ShouldBeTrue)
						})
					} else {
						Convey("the variant-specific function command should not run", func() {
							So(scanLogsForTask(modelData.Task.Id, "", "variant not excluded!"), ShouldBeFalse)
							So(scanLogsForTask(modelData.Task.Id, "", "Skipping command 'shell.exec'"), ShouldBeTrue)
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
			modelData, err := modelutil.SetupAPITestData(testConfig, "timeout_task", "linux-64",
				filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			assignAgentTask(testAgent, modelData.Task)
			prependConfigToVersion(t, modelData.Task.Version, "callback_timeout_secs: 1\n")

			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(7 * time.Second)
				So(testAgent.taskConfig.Project.CallbackTimeout, ShouldEqual, 1)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err := task.FindOne(task.ById(modelData.Task.Id))
					So(err, ShouldBeNil)
					So(testTask.Status, ShouldEqual, evergreen.TaskFailed)
					So(testTask.Details.TimedOut, ShouldBeTrue)
					So(testTask.Details.Description, ShouldEqual, "shell.exec")

					Convey("and the timeout task should have failed", func() {
						So(scanLogsForTask(modelData.Task.Id, "", "running task-timeout commands in 1"), ShouldBeTrue)
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
			modelData, err := modelutil.SetupAPITestData(testConfig, "exec_timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test task")
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()
			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			assignAgentTask(testAgent, modelData.Task)
			Convey("after the slow test runs beyond the timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(modelData.Task.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err := task.FindOne(task.ById(modelData.Task.Id))
					So(err, ShouldBeNil)
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
			modelData, err := modelutil.SetupAPITestData(testConfig, "project_exec_timeout_task", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/project-timeout-test.yml"), modelutil.NoPatch)
			testutil.HandleTestingErr(err, t, "Failed to find test task")

			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			assignAgentTask(testAgent, modelData.Task)

			Convey("after the slow test runs beyond the project timeout threshold", func() {
				// actually run the task.
				// this function won't return until the whole thing is done.
				_, err = testAgent.RunTask()
				So(err, ShouldBeNil)
				testAgent.APILogger.Flush()
				time.Sleep(5 * time.Second)
				printLogsForTask(modelData.Task.Id)
				Convey("the test should be marked as failed and timed out", func() {
					So(scanLogsForTask(modelData.Task.Id, "", "executing the pre-run script"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the post-run script!"), ShouldBeTrue)
					So(scanLogsForTask(modelData.Task.Id, "", "executing the task-timeout script!"), ShouldBeTrue)
					testTask, err := task.FindOne(task.ById(modelData.Task.Id))
					So(err, ShouldBeNil)
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
		modelData, err := modelutil.SetupAPITestData(testConfig, "random", "linux-64", filepath.Join(testDirectory, "testdata/config_test_plugin/project/evergreen-ci-render.yml"), modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "Couldn't make test data: %v", err)

		Convey("With a live api server, agent, and test task over "+tlsString, t, func() {
			testServer, err := service.CreateTestServer(testConfig, tlsConfig, plugin.APIPlugins)
			testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
			defer testServer.Close()

			testAgent, err := createAgent(testServer, modelData.Host)
			testutil.HandleTestingErr(err, t, "failed to create agent: %v")

			assignAgentTask(testAgent, modelData.Task)

			testAgent.heartbeater.Interval = 10 * time.Second
			testAgent.StartBackgroundActions(&NoopSignalHandler{})

			Convey("calling end() should update task's/host's status properly ", func() {
				details := &apimodels.TaskEndDetail{Status: evergreen.TaskSucceeded}
				taskEndResp, err := testAgent.End(details)
				time.Sleep(1 * time.Second)
				So(err, ShouldBeNil)
				So(taskEndResp.ShouldExit, ShouldBeFalse)

				taskUpdate, err := task.FindOne(task.ById(modelData.Task.Id))
				So(err, ShouldBeNil)
				So(taskUpdate.Status, ShouldEqual, evergreen.TaskSucceeded)

				testHost, err := host.FindOne(host.ById(modelData.Task.HostId))
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

type patchRequest struct {
	moduleName string
	filePath   string
	githash    string
}

func setupPatches(patchMode modelutil.PatchTestMode, b *build.Build, t *testing.T, patches ...patchRequest) {
	if patchMode == modelutil.NoPatch {
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

		if patchMode == modelutil.InlinePatch {
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
