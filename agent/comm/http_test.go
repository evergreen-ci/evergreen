package comm

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/tychoish/grip/send"
	"github.com/tychoish/grip/slogger"
)

func TestCommunicatorServerDown(t *testing.T) {
	Convey("With an HTTP communicator and a dead HTTP server", t, func() {
		logger := &slogger.Logger{"test", []send.Sender{slogger.StdOutAppender()}}
		downServer := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		)
		downServer.Close()

		agentCommunicator := HTTPCommunicator{
			ServerURLRoot:   downServer.URL,         // root URL of api server
			TaskId:          "mocktaskid",           // task ID
			TaskSecret:      "mocktasksecret",       // task Secret
			MaxAttempts:     3,                      // max # of retries for each API call
			RetrySleep:      100 * time.Millisecond, // sleep time between API call retries
			SignalChan:      make(chan Signal),      // channel for agent signals
			Logger:          logger,                 // logger to use for logging retry attempts
			HttpsCert:       "",                     // cert
			httpClient:      &http.Client{},
			heartbeatClient: &http.Client{},
		}
		Convey("Calling start() should return err after max retries", func() {
			So(agentCommunicator.Start("1"), ShouldNotBeNil)
		})

		Convey("Calling GetTask() should return err after max retries", func() {
			agentTestTask, err := agentCommunicator.GetTask()
			So(err, ShouldNotBeNil)
			So(agentTestTask, ShouldBeNil)
		})
	})
}

func TestCommunicatorServerUp(t *testing.T) {
	Convey("With an HTTP communicator and live HTTP server", t, func() {
		serveMux := http.NewServeMux()
		ts := httptest.NewServer(serveMux)
		logger := &slogger.Logger{"test", []send.Sender{slogger.StdOutAppender()}}

		agentCommunicator := HTTPCommunicator{
			ServerURLRoot:   ts.URL,
			TaskId:          "mocktaskid",
			TaskSecret:      "mocktasksecret",
			MaxAttempts:     3,
			RetrySleep:      100 * time.Millisecond,
			SignalChan:      make(chan Signal),
			Logger:          logger,
			HttpsCert:       "",
			httpClient:      &http.Client{},
			heartbeatClient: &http.Client{},
		}

		Convey("Calls to start() or end() should not return err", func() {
			// Mock start/end handlers to answer the agent's requests
			serveMux.HandleFunc("/task/mocktaskid/start",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, apimodels.TaskStartRequest{}, http.StatusOK)
				})
			serveMux.HandleFunc("/task/mocktaskid/end",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, apimodels.TaskEndResponse{}, http.StatusOK)
				})

			So(agentCommunicator.Start("1"), ShouldBeNil)
			details := &apimodels.TaskEndDetail{Status: evergreen.TaskFailed}
			_, err := agentCommunicator.End(details)
			So(err, ShouldBeNil)
		})

		Convey("Calls to GetVersion() should return the right version", func() {
			v := &version.Version{
				Author: "hey wow",
				Config: "enabled: true\nbatchtime: 120",
			}
			// Mock project handler to answer the agent's request for a
			// project's config
			serveMux.HandleFunc("/task/mocktaskid/version",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, v, http.StatusOK)
				})
			v, err := agentCommunicator.GetVersion()
			So(err, ShouldBeNil)
			So(v.Author, ShouldEqual, "hey wow")
			So(v.Config, ShouldNotEqual, "")
		})
		Convey("Calls to GetProjectRefConfig() should return the right config", func() {
			v := &model.ProjectRef{
				Identifier: "mock_identifier",
				Enabled:    true,
				BatchTime:  120,
			}
			// Mock project handler to answer the agent's request for a
			// project's config
			serveMux.HandleFunc("/task/mocktaskid/project_ref",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, v, http.StatusOK)
				})
			projectConfig, err := agentCommunicator.GetProjectRef()
			So(err, ShouldBeNil)

			So(projectConfig.Identifier, ShouldEqual, "mock_identifier")
			So(projectConfig.Enabled, ShouldBeTrue)
			So(projectConfig.BatchTime, ShouldEqual, 120)
		})

		Convey("Calling GetTask() should fetch the task successfully", func() {
			testTask := &task.Task{Id: "mocktaskid"}
			serveMux.HandleFunc("/task/mocktaskid/",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, testTask, http.StatusOK)
				})

			agentTestTask, err := agentCommunicator.GetTask()
			So(err, ShouldBeNil)
			So(agentTestTask, ShouldNotBeNil)
			So(agentTestTask.Id, ShouldEqual, testTask.Id)
		})

		Convey("Calling GetDistro() should fetch the task's distro successfully",
			func() {
				d := &distro.Distro{Id: "mocktaskdistro"}
				serveMux.HandleFunc("/task/mocktaskid/distro",
					func(w http.ResponseWriter, req *http.Request) {
						util.WriteJSON(&w, d, http.StatusOK)
					})

				d, err := agentCommunicator.GetDistro()
				So(err, ShouldBeNil)
				So(d, ShouldNotBeNil)
				So(d.Id, ShouldEqual, "mocktaskdistro")
			})

		Convey("Failed calls to start() or end() should retry till success", func() {
			startCount := 0
			endCount := 0

			// Use mock start and end handlers which will succeed only after
			// a certain number of requests have been made.
			serveMux.HandleFunc("/task/mocktaskid/start",
				func(w http.ResponseWriter, req *http.Request) {
					startCount++
					if startCount == 3 {
						util.WriteJSON(&w, apimodels.TaskStartRequest{}, http.StatusOK)
					} else {
						util.WriteJSON(&w, apimodels.TaskEndResponse{}, http.StatusInternalServerError)
					}
				})
			serveMux.HandleFunc("/task/mocktaskid/end",
				func(w http.ResponseWriter, req *http.Request) {
					endCount++
					if endCount == 3 {
						util.WriteJSON(&w, apimodels.TaskEndResponse{}, http.StatusOK)
					} else {
						util.WriteJSON(&w, apimodels.TaskEndResponse{}, http.StatusInternalServerError)
					}
				})
			So(agentCommunicator.Start("1"), ShouldBeNil)
			details := &apimodels.TaskEndDetail{Status: evergreen.TaskFailed}
			_, err := agentCommunicator.End(details)
			So(err, ShouldBeNil)
		})

		Convey("With an agent sending calls to the heartbeat endpoint", func() {
			heartbeatFail := true
			heartbeatAbort := false
			serveMux.HandleFunc("/task/mocktaskid/heartbeat", func(w http.ResponseWriter, req *http.Request) {
				if heartbeatFail {
					util.WriteJSON(&w, apimodels.HeartbeatResponse{}, http.StatusInternalServerError)
				} else {
					util.WriteJSON(&w, apimodels.HeartbeatResponse{heartbeatAbort}, http.StatusOK)
				}
			})
			Convey("Failing calls should return err and successful calls should not", func() {
				_, err := agentCommunicator.Heartbeat()
				So(err, ShouldNotBeNil)

				heartbeatFail = false
				_, err = agentCommunicator.Heartbeat()
				So(err, ShouldBeNil)

				Convey("Heartbeat calls should detect aborted tasks", func() {
					heartbeatAbort = true
					abortflag, err := agentCommunicator.Heartbeat()
					So(err, ShouldBeNil)
					So(abortflag, ShouldBeTrue)
				})
			})
		})

		Convey("Calling Log() should serialize/deserialize correctly", func() {
			outgoingMessages := []model.LogMessage{
				{"S", "E", "message1", time.Now(), evergreen.LogmessageCurrentVersion},
				{"S", "E", "message2", time.Now(), evergreen.LogmessageCurrentVersion},
				{"S", "E", "message3", time.Now(), evergreen.LogmessageCurrentVersion},
				{"S", "E", "message4", time.Now(), evergreen.LogmessageCurrentVersion},
				{"S", "E", "message5", time.Now(), evergreen.LogmessageCurrentVersion},
			}
			incoming := &model.TaskLog{}
			serveMux.HandleFunc("/task/mocktaskid/log",
				func(w http.ResponseWriter, req *http.Request) {
					util.ReadJSONInto(ioutil.NopCloser(req.Body), incoming)
				})
			err := agentCommunicator.Log(outgoingMessages)
			So(err, ShouldBeNil)
			time.Sleep(10 * time.Millisecond)
			for index := range outgoingMessages {
				So(incoming.Messages[index].Type, ShouldEqual, outgoingMessages[index].Type)
				So(incoming.Messages[index].Severity, ShouldEqual, outgoingMessages[index].Severity)
				So(incoming.Messages[index].Message, ShouldEqual, outgoingMessages[index].Message)
				So(incoming.Messages[index].Timestamp.Equal(outgoingMessages[index].Timestamp), ShouldBeTrue)
			}
		})

		Convey("fetching expansions should work", func() {
			test_vars := apimodels.ExpansionVars{}
			test_vars["test_key"] = "test_value"
			test_vars["second_fetch"] = "more_one"
			serveMux.HandleFunc("/task/mocktaskid/fetch_vars", func(w http.ResponseWriter, req *http.Request) {
				util.WriteJSON(&w, test_vars, http.StatusOK)
			})
			resultingVars, err := agentCommunicator.FetchExpansionVars()
			So(err, ShouldBeNil)
			So(len(*resultingVars), ShouldEqual, 2)
			So((*resultingVars)["test_key"], ShouldEqual, "test_value")
			So((*resultingVars)["second_fetch"], ShouldEqual, "more_one")

		})
	})
}
