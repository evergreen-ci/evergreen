package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAgentRun(t *testing.T) {
	Convey("with a HTTP communicator and an agent that uses it", t, func() {
		serveMux := http.NewServeMux()
		ts := httptest.NewServer(serveMux)

		hostId := "host"
		hostSecret := "secret"
		agentCommunicator, err := comm.NewHTTPCommunicator(
			ts.URL, hostId, hostSecret, "", make(chan comm.Signal))
		So(err, ShouldBeNil)

		testAgent := Agent{
			TaskCommunicator: agentCommunicator,
		}

		Convey("with a response that indicates that the agent should exit", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "mocktaskid",
				TaskSecret: "secret",
				ShouldExit: true}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			So(testAgent.Run(), ShouldNotBeNil)
		})
		Convey("with a task without a secret", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "mocktaskid",
				TaskSecret: "",
				ShouldExit: false}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			So(testAgent.Run(), ShouldNotBeNil)
		})

	})
}

func TestAgentGetNextTask(t *testing.T) {
	Convey("with a HTTP communicator and an agent that uses it", t, func() {
		serveMux := http.NewServeMux()
		ts := httptest.NewServer(serveMux)

		hostId := "host"
		hostSecret := "secret"
		agentCommunicator, err := comm.NewHTTPCommunicator(
			ts.URL, hostId, hostSecret, "", make(chan comm.Signal))
		So(err, ShouldBeNil)

		testAgent := Agent{
			TaskCommunicator: agentCommunicator,
		}
		Convey("with a response that returns a task id and task secret", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "mocktaskid",
				TaskSecret: "secret",
				ShouldExit: false}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			hasTask, err := testAgent.getNextTask()
			So(err, ShouldBeNil)
			So(hasTask, ShouldBeTrue)
			httpTaskComm, ok := testAgent.TaskCommunicator.(*comm.HTTPCommunicator)
			So(ok, ShouldBeTrue)
			So(httpTaskComm.TaskId, ShouldEqual, "mocktaskid")
			So(httpTaskComm.TaskSecret, ShouldEqual, "secret")
		})
		Convey("with a response that should exit", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "mocktaskid",
				TaskSecret: "secret",
				ShouldExit: true}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			hasTask, err := testAgent.getNextTask()
			So(err, ShouldNotBeNil)
			So(hasTask, ShouldBeFalse)
		})
		Convey("with a response that does not have a task id", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "",
				TaskSecret: "",
				ShouldExit: false}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			hasTask, err := testAgent.getNextTask()
			So(err, ShouldBeNil)
			So(hasTask, ShouldBeFalse)
		})
		Convey("with a response that does not have a secret but has a task id", func() {
			resp := &apimodels.NextTaskResponse{
				TaskId:     "taskid",
				TaskSecret: "",
				ShouldExit: false}

			serveMux.HandleFunc("/api/2/agent/next_task",
				func(w http.ResponseWriter, req *http.Request) {
					util.WriteJSON(&w, resp, http.StatusOK)
				})
			hasTask, err := testAgent.getNextTask()
			So(err, ShouldNotBeNil)
			So(hasTask, ShouldBeFalse)
		})

	})
}
