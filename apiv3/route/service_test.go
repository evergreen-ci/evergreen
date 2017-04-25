package route

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/context"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHostPaginator(t *testing.T) {
	numHostsInDB := 300
	Convey("When paginating with a ServiceContext", t, func() {
		serviceContext := servicecontext.MockServiceContext{}
		Convey("and there are hosts to be found", func() {
			cachedHosts := []host.Host{}
			for i := 0; i < numHostsInDB; i++ {
				nextHost := host.Host{
					Id: fmt.Sprintf("host%d", i),
				}
				cachedHosts = append(cachedHosts, nextHost)
			}
			serviceContext.MockHostConnector.CachedHosts = cachedHosts
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				hostToStartAt := 100
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				hostToStartAt := 150
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    50,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				hostToStartAt := 50
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", 0),
						Limit:    50,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the last page should produce only a previous"+
				" page and a limited set of models", func() {
				hostToStartAt := 299
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < numHostsInDB; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				hostToStartAt := 0
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
		})
	})
}

func TestTaskExecutionPatchPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		tep := &TaskExecutionPatchHandler{}

		projCtx := serviceModel.Context{
			Task: &task.Task{
				Id:        "testTaskId",
				Priority:  0,
				Activated: false,
			},
		}
		u := user.DBUser{
			Id: "testUser",
		}
		Convey("then should error on empty body", func() {
			req, err := http.NewRequest("PATCH", "task/testTaskId", &bytes.Buffer{})
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = tep.ParseAndValidate(req)
			So(err, ShouldNotBeNil)
			expectedErr := apiv3.APIError{
				Message:    "No request body sent",
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should error on body with wrong type", func() {
			str := "nope"
			badBod := &struct {
				Activated *string
			}{
				Activated: &str,
			}
			res, err := json.Marshal(badBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = tep.ParseAndValidate(req)
			So(err, ShouldNotBeNil)
			expectedErr := apiv3.APIError{
				Message: fmt.Sprintf("Incorrect type given, expecting '%s' "+
					"but receieved '%s'",
					"bool", "string"),
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should error when fields not set", func() {
			badBod := &struct {
				Activated *string
			}{}
			res, err := json.Marshal(badBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = tep.ParseAndValidate(req)
			So(err, ShouldNotBeNil)
			expectedErr := apiv3.APIError{
				Message:    "Must set 'activated' or 'priority'",
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should set it's Activated and Priority field when set", func() {
			goodBod := &struct {
				Activated bool
				Priority  int
			}{
				Activated: true,
				Priority:  100,
			}
			res, err := json.Marshal(goodBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = tep.ParseAndValidate(req)
			So(err, ShouldBeNil)
			So(*tep.Activated, ShouldBeTrue)
			So(*tep.Priority, ShouldEqual, 100)

			Convey("and task and user should be set", func() {
				So(tep.task, ShouldNotBeNil)
				So(tep.task.Id, ShouldEqual, "testTaskId")
				So(tep.user.Username(), ShouldEqual, "testUser")
			})
		})
	})
}

func TestTaskExecutionPatchExecute(t *testing.T) {
	Convey("With a task in the DB and a ServiceContext", t, func() {
		sc := servicecontext.MockServiceContext{}
		testTask := task.Task{
			Id:        "testTaskId",
			Activated: false,
			Priority:  10,
		}
		sc.MockTaskConnector.CachedTasks = append(sc.MockTaskConnector.CachedTasks, testTask)

		Convey("then setting priority should change it's priority", func() {
			act := true
			var prio int64 = 100

			tep := &TaskExecutionPatchHandler{
				Activated: &act,
				Priority:  &prio,
				task: &task.Task{
					Id: "testTaskId",
				},
				user: &user.DBUser{
					Id: "testUser",
				},
			}
			res, err := tep.Execute(&sc)
			So(err, ShouldBeNil)
			So(len(res.Result), ShouldEqual, 1)
			resModel := res.Result[0]
			resTask, ok := resModel.(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Priority, ShouldEqual, int64(100))
			So(resTask.Activated, ShouldBeTrue)
			So(resTask.ActivatedBy, ShouldEqual, "testUser")
		})
	})
}

func TestTaskResetPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		trh := &TaskRestartHandler{}

		projCtx := serviceModel.Context{
			Task: &task.Task{
				Id:        "testTaskId",
				Priority:  0,
				Activated: false,
			},
			Project: &serviceModel.Project{},
		}
		u := user.DBUser{
			Id: "testUser",
		}
		Convey("should error on empty project", func() {
			projCtx.Project = nil
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = trh.ParseAndValidate(req)
			So(err, ShouldNotBeNil)
			expectedErr := fmt.Errorf("Unable to fetch associated project")
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should error on empty task", func() {
			projCtx.Task = nil
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			context.Set(req, RequestUser, &u)
			context.Set(req, RequestContext, &projCtx)
			err = trh.ParseAndValidate(req)
			So(err, ShouldNotBeNil)
			expectedErr := apiv3.APIError{
				Message:    "Task not found",
				StatusCode: http.StatusNotFound,
			}

			So(err, ShouldResemble, expectedErr)
		})
	})
}

func TestTaskResetExecute(t *testing.T) {
	Convey("With a task returned by the ServiceContext", t, func() {
		sc := servicecontext.MockServiceContext{}
		timeNow := time.Now()
		testTask := task.Task{
			Id:           "testTaskId",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: timeNow,
		}
		sc.MockTaskConnector.CachedTasks = append(sc.MockTaskConnector.CachedTasks, testTask)
		Convey("and an error from the service function", func() {
			sc.MockTaskConnector.StoredError = fmt.Errorf("could not reset task")

			trh := &TaskRestartHandler{
				taskId:   "testTaskId",
				project:  &serviceModel.Project{},
				username: "testUser",
			}

			_, err := trh.Execute(&sc)
			So(err, ShouldNotBeNil)
			apiErr, ok := err.(apiv3.APIError)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusBadRequest)

		})

		Convey("calling TryReset should reset the task", func() {

			trh := &TaskRestartHandler{
				taskId:   "testTaskId",
				project:  &serviceModel.Project{},
				username: "testUser",
			}

			res, err := trh.Execute(&sc)
			So(err, ShouldBeNil)
			So(len(res.Result), ShouldEqual, 1)
			resModel := res.Result[0]
			resTask, ok := resModel.(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Activated, ShouldBeTrue)
			So(time.Time(resTask.DispatchTime), ShouldResemble, util.ZeroTime)
			dbTask, err := sc.FindTaskById("testTaskId")
			So(err, ShouldBeNil)
			So(string(dbTask.Secret), ShouldNotResemble, "initialSecret")
		})
	})

}

func checkPaginatorResultMatches(paginator PaginatorFunc, key string, limit int,
	sc servicecontext.ServiceContext, expectedPages *PageResult,
	expectedModels []model.Model, expectedErr error) {
	res, pages, err := paginator(key, limit, sc)
	So(err, ShouldEqual, expectedErr)
	So(res, ShouldResemble, expectedModels)
	So(pages, ShouldResemble, expectedPages)
}
