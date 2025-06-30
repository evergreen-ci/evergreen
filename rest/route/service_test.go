package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
	config := evergreen.NaiveAuthConfig{}
	um, err := auth.NewNaiveUserManager(&config)
	grip.EmergencyFatal(err)
	evergreen.GetEnvironment().SetUserManager(um)
}

func TestHostParseAndValidate(t *testing.T) {
	Convey("With a hostGetHandler and request", t, func() {
		testStatus := "testStatus"
		hgh := &hostGetHandler{}
		hgh, ok := hgh.Factory().(*hostGetHandler)
		So(ok, ShouldBeTrue)
		u := url.URL{
			RawQuery: fmt.Sprintf("status=%s", testStatus),
		}
		r := http.Request{
			URL: &u,
		}
		ctx := context.Background()

		Convey("parsing request should fetch status", func() {
			err := hgh.Parse(ctx, &r)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
			So(hgh.status, ShouldEqual, testStatus)
		})
	})
}

func TestHostPaginator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numHostsInDB := 300
	Convey("When paginating with a Connector", t, func() {
		So(db.Clear(host.Collection), ShouldBeNil)
		Convey("and there are hosts to be found", func() {
			cachedHosts := []host.Host{}
			for i := 0; i < numHostsInDB; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextHost := host.Host{
					Id: fmt.Sprintf("%dhost%d", prefix, i),
					Distro: distro.Distro{
						Provider: evergreen.ProviderNameMock,
					},
					Status: evergreen.HostRunning,
				}
				cachedHosts = append(cachedHosts, nextHost)
				So(nextHost.Insert(ctx), ShouldBeNil)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				hostToStartAt := 100
				limit := 100
				expectedHosts := []any{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:                utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL:           utility.ToStringPtr(""),
						PersistentDNSName: utility.ToStringPtr(""),
						Distro: model.DistroInfo{
							Id:                   utility.ToStringPtr(""),
							Provider:             utility.ToStringPtr(evergreen.ProviderNameMock),
							ImageId:              utility.ToStringPtr(""),
							WorkDir:              utility.ToStringPtr(""),
							IsVirtualWorkstation: false,
							User:                 utility.ToStringPtr(""),
							BootstrapMethod:      utility.ToStringPtr(""),
						},
						StartedBy:         utility.ToStringPtr(""),
						Provider:          utility.ToStringPtr(""),
						User:              utility.ToStringPtr(""),
						Status:            utility.ToStringPtr(evergreen.HostRunning),
						NeedsReprovision:  utility.ToStringPtr(""),
						InstanceType:      utility.ToStringPtr(""),
						AvailabilityZone:  utility.ToStringPtr(""),
						DisplayName:       utility.ToStringPtr(""),
						HomeVolumeID:      utility.ToStringPtr(""),
						Tag:               utility.ToStringPtr(""),
						AttachedVolumeIDs: []string{},
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				prefix := int(math.Log10(float64(hostToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dhost%d", prefix, hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
					url:   "http://evergreen.example.net",
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				hostToStartAt := 150
				limit := 100
				expectedHosts := []any{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:                utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL:           utility.ToStringPtr(""),
						PersistentDNSName: utility.ToStringPtr(""),
						Distro: model.DistroInfo{
							Id:                   utility.ToStringPtr(""),
							Provider:             utility.ToStringPtr(evergreen.ProviderNameMock),
							ImageId:              utility.ToStringPtr(""),
							WorkDir:              utility.ToStringPtr(""),
							IsVirtualWorkstation: false,
							User:                 utility.ToStringPtr(""),
							BootstrapMethod:      utility.ToStringPtr(""),
						},
						StartedBy:         utility.ToStringPtr(""),
						Provider:          utility.ToStringPtr(""),
						User:              utility.ToStringPtr(""),
						Status:            utility.ToStringPtr(evergreen.HostRunning),
						NeedsReprovision:  utility.ToStringPtr(""),
						InstanceType:      utility.ToStringPtr(""),
						AvailabilityZone:  utility.ToStringPtr(""),
						DisplayName:       utility.ToStringPtr(""),
						HomeVolumeID:      utility.ToStringPtr(""),
						Tag:               utility.ToStringPtr(""),
						AttachedVolumeIDs: []string{},
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				prefix := int(math.Log10(float64(hostToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dhost%d", prefix, hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
					url:   "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				hostToStartAt := 50
				limit := 100
				expectedHosts := []any{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:                utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL:           utility.ToStringPtr(""),
						PersistentDNSName: utility.ToStringPtr(""),
						Distro: model.DistroInfo{
							Id:                   utility.ToStringPtr(""),
							Provider:             utility.ToStringPtr(evergreen.ProviderNameMock),
							ImageId:              utility.ToStringPtr(""),
							WorkDir:              utility.ToStringPtr(""),
							IsVirtualWorkstation: false,
							User:                 utility.ToStringPtr(""),
							BootstrapMethod:      utility.ToStringPtr(""),
						},
						StartedBy:         utility.ToStringPtr(""),
						Provider:          utility.ToStringPtr(""),
						User:              utility.ToStringPtr(""),
						Status:            utility.ToStringPtr(evergreen.HostRunning),
						NeedsReprovision:  utility.ToStringPtr(""),
						InstanceType:      utility.ToStringPtr(""),
						AvailabilityZone:  utility.ToStringPtr(""),
						DisplayName:       utility.ToStringPtr(""),
						HomeVolumeID:      utility.ToStringPtr(""),
						Tag:               utility.ToStringPtr(""),
						AttachedVolumeIDs: []string{},
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				prefix := int(math.Log10(float64(hostToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dhost%d", prefix, hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
					url:   "http://evergreen.example.net",
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				hostToStartAt := 0
				limit := 100
				expectedHosts := []any{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:                utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL:           utility.ToStringPtr(""),
						PersistentDNSName: utility.ToStringPtr(""),
						Distro: model.DistroInfo{
							Id:                   utility.ToStringPtr(""),
							Provider:             utility.ToStringPtr(evergreen.ProviderNameMock),
							ImageId:              utility.ToStringPtr(""),
							WorkDir:              utility.ToStringPtr(""),
							IsVirtualWorkstation: false,
							User:                 utility.ToStringPtr(""),
							BootstrapMethod:      utility.ToStringPtr(""),
						},
						StartedBy:         utility.ToStringPtr(""),
						Provider:          utility.ToStringPtr(""),
						User:              utility.ToStringPtr(""),
						Status:            utility.ToStringPtr(evergreen.HostRunning),
						NeedsReprovision:  utility.ToStringPtr(""),
						InstanceType:      utility.ToStringPtr(""),
						AvailabilityZone:  utility.ToStringPtr(""),
						DisplayName:       utility.ToStringPtr(""),
						HomeVolumeID:      utility.ToStringPtr(""),
						Tag:               utility.ToStringPtr(""),
						AttachedVolumeIDs: []string{},
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				prefix := int(math.Log10(float64(hostToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dhost%d", prefix, hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
					url:   "http://evergreen.example.net",
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
		})
	})
}

func TestTasksByProjectAndCommitPaginator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numTasks := 300
	projectId := "project_1"
	projectName := "project_identifier"
	commit := "commit_1"
	Convey("When paginating with a Connector", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, serviceModel.ProjectRefCollection))
		p := &serviceModel.ProjectRef{
			Id:         projectId,
			Identifier: projectName,
		}
		assert.NoError(t, p.Insert(t.Context()))
		Convey("and there are tasks to be found", func() {
			cachedTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextTask := task.Task{
					Id:        fmt.Sprintf("%dtask_%d", prefix, i),
					Revision:  commit,
					Project:   projectId,
					Requester: evergreen.RepotrackerVersionRequester,
				}
				cachedTasks = append(cachedTasks, nextTask)
			}
			for _, cachedTask := range cachedTasks {
				err := db.Insert(t.Context(), task.Collection, cachedTask)
				So(err, ShouldBeNil)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:        fmt.Sprintf("%dtask_%d", prefix, i),
						Revision:  commit,
						Project:   projectId,
						Requester: evergreen.RepotrackerVersionRequester,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dtask_%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						LimitQueryParam: "limit",
						KeyQueryParam:   "start_at",
					},
				}
				handler := &tasksByProjectHandler{
					project:    projectId,
					commitHash: commit,
					key:        fmt.Sprintf("%dtask_%d", prefix, taskToStartAt),
					limit:      limit,
					url:        "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, handler, expectedTasks, expectedPages)
			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				taskToStartAt := 150
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:        fmt.Sprintf("%dtask_%d", prefix, i),
						Revision:  commit,
						Project:   projectId,
						Requester: evergreen.RepotrackerVersionRequester,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceTask, &model.APITaskArgs{
						LogURL:                   "http://evergreen.example.net",
						ParsleyLogURL:            "http://parsley.example.net",
						IncludeProjectIdentifier: true,
					})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dtask_%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						LimitQueryParam: "limit",
						KeyQueryParam:   "start_at",
					},
				}
				handler := &tasksByProjectHandler{
					project:    projectName,
					commitHash: commit,
					key:        fmt.Sprintf("%dtask_%d", prefix, taskToStartAt),
					limit:      limit,
					url:        "http://evergreen.example.net",
					parsleyURL: "http://parsley.example.net",
				}

				validatePaginatedResponse(t, handler, expectedTasks, expectedPages)
			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:        fmt.Sprintf("%dtask_%d", prefix, i),
						Revision:  commit,
						Project:   projectId,
						Requester: evergreen.RepotrackerVersionRequester,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dtask_%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						LimitQueryParam: "limit",
						KeyQueryParam:   "start_at",
						BaseURL:         "http://evergreen.example.net",
						Relation:        "next",
					},
				}
				prefix = int(math.Log10(float64(taskToStartAt)))
				handler := &tasksByProjectHandler{
					project:    projectId,
					commitHash: commit,
					key:        fmt.Sprintf("%dtask_%d", prefix, taskToStartAt),
					limit:      limit,
					url:        "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, handler, expectedTasks, expectedPages)
			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				taskToStartAt := 0
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:        fmt.Sprintf("%dtask_%d", prefix, i),
						Revision:  commit,
						Project:   projectId,
						Requester: evergreen.RepotrackerVersionRequester,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dtask_%d", prefix, taskToStartAt+limit),
						LimitQueryParam: "limit",
						KeyQueryParam:   "start_at",
						Limit:           limit,
						BaseURL:         "http://evergreen.example.net",
						Relation:        "next",
					},
				}

				handler := &tasksByProjectHandler{
					project:    projectName,
					commitHash: commit,
					key:        fmt.Sprintf("%dtask_%d", 0, taskToStartAt),
					limit:      limit,
					url:        "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, handler, expectedTasks, expectedPages)
			})
		})
	})
}

func TestTaskByProjectHandlerParse(t *testing.T) {
	Convey("Parsing get task by projects handler", t, func() {
		tep := &tasksByProjectHandler{}
		ctx := context.Background()
		Convey("should successfully parse with no url query and default requester to mainline", func() {
			req, err := http.NewRequest(http.MethodGet, "/projects/evergreen/revisions/hash123/tasks", bytes.NewReader(nil))
			req = gimlet.SetURLVars(req, map[string]string{"commit_hash": "hash123", "project_id": "evergreen"})
			So(err, ShouldBeNil)
			err = tep.Parse(ctx, req)
			So(err, ShouldBeNil)
			So(tep.project, ShouldEqual, "evergreen")
			So(tep.commitHash, ShouldEqual, "hash123")
		})
		Convey("should successfully parse all query params", func() {
			req, err := http.NewRequest(http.MethodGet, "/projects/evergreen/revisions/hash123/tasks?status=succeeded&variant=ubuntu1604&limit=200&task_name=task1&requesters=gitter_request,github_pull_request", bytes.NewReader(nil))
			So(err, ShouldBeNil)
			req = gimlet.SetURLVars(req, map[string]string{"commit_hash": "hash123", "project_id": "evergreen"})
			err = tep.Parse(ctx, req)
			So(err, ShouldBeNil)
			So(tep.variant, ShouldEqual, "ubuntu1604")
			So(tep.taskName, ShouldEqual, "task1")
			So(tep.status, ShouldEqual, "succeeded")
		})
		Convey("should fail on missing project or commit hash", func() {
			req, err := http.NewRequest(http.MethodGet, "/projects/evergreen/revisions/hash123/tasks?status=succeeded&variant=ubuntu1604&limit=200&task_name=task1&requesters=gitter_request,github_pull_request", bytes.NewReader(nil))
			So(err, ShouldBeNil)
			req = gimlet.SetURLVars(req, map[string]string{"commit_hash": "hash123"})
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			req = gimlet.SetURLVars(req, map[string]string{"project_id": "evergreen"})
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
		})
		Convey("should fail on invalid limit", func() {
			req, err := http.NewRequest(http.MethodGet, "/projects/evergreen/revisions/hash123/tasks?limit=abc", bytes.NewReader(nil))
			So(err, ShouldBeNil)
			req = gimlet.SetURLVars(req, map[string]string{"commit_hash": "hash123", "project_id": "evergreen"})
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTaskByBuildPaginator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numTasks := 300
	Convey("When paginating with a Connector", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection))
		Convey("and there are tasks to be found", func() {
			cachedOldTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextTask := task.Task{
					Id: fmt.Sprintf("%dbuild%d", prefix, i),
				}
				So(db.Insert(t.Context(), task.Collection, nextTask), ShouldBeNil)
			}
			for i := 0; i < 5; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextTask := task.Task{
					Id:        fmt.Sprintf("%dbuild0_%d", prefix, i),
					OldTaskId: "0build0",
					Execution: i,
				}
				So(db.Insert(t.Context(), task.OldCollection, nextTask), ShouldBeNil)
				cachedOldTasks = append(cachedOldTasks, nextTask)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceModel, &model.APITaskArgs{
						LogURL:                   "http://evergreen.example.net",
						ParsleyLogURL:            "http://parsley.example.net",
						IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dbuild%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}
				prefix = int(math.Log10(float64(taskToStartAt)))
				tbh := &tasksByBuildHandler{
					limit:      limit,
					key:        fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:        "http://evergreen.example.net",
					parsleyURL: "http://parsley.example.net",
				}

				// SPARTA
				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				taskToStartAt := 150
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceModel, &model.APITaskArgs{
						LogURL:                   "http://evergreen.example.net",
						ParsleyLogURL:            "http://parsley.example.net",
						IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dbuild%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				prefix = int(math.Log10(float64(taskToStartAt)))
				tbh := &tasksByBuildHandler{
					limit:      limit,
					key:        fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:        "http://evergreen.example.net",
					parsleyURL: "http://parsley.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceModel, &model.APITaskArgs{
						LogURL:                   "http://evergreen.example.net",
						ParsleyLogURL:            "http://parsley.example.net",
						IncludeProjectIdentifier: true,
					})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dbuild%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}
				prefix = int(math.Log10(float64(taskToStartAt)))
				tbh := &tasksByBuildHandler{
					limit:      limit,
					key:        fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:        "http://evergreen.example.net",
					parsleyURL: "http://parsley.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				taskToStartAt := 0
				limit := 100
				expectedTasks := []any{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(ctx, serviceModel, &model.APITaskArgs{
						LogURL:                   "http://evergreen.example.net",
						ParsleyLogURL:            "http://parsley.example.net",
						IncludeProjectIdentifier: true})
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				prefix := int(math.Log10(float64(taskToStartAt + limit)))
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("%dbuild%d", prefix, taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit:      limit,
					key:        fmt.Sprintf("%dbuild%d", 0, taskToStartAt),
					url:        "http://evergreen.example.net",
					parsleyURL: "http://parsley.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("pagination with tasks with previous executions", func() {
				expectedTasks := []any{}
				serviceModel := &task.Task{
					Id: "0build0",
				}
				nextModelTask := &model.APITask{}
				err := nextModelTask.BuildFromService(ctx, serviceModel, &model.APITaskArgs{
					LogURL:                   "http://evergreen.example.net",
					ParsleyLogURL:            "http://parsley.example.net",
					IncludeProjectIdentifier: true,
				})
				So(err, ShouldBeNil)
				err = nextModelTask.BuildPreviousExecutions(ctx, cachedOldTasks, "http://evergreen.example.net", "http://parsley.example.net")
				So(err, ShouldBeNil)
				expectedTasks = append(expectedTasks, nextModelTask)
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             "0build1",
						Limit:           1,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit:              1,
					key:                "0build0",
					fetchAllExecutions: true,
					url:                "http://evergreen.example.net",
					parsleyURL:         "http://parsley.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})
		})
	})
}

func TestTaskExecutionPatchPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		tep := &taskExecutionPatchHandler{}

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
		ctx := context.Background()
		Convey("then should error on empty body", func() {
			req, err := http.NewRequest(http.MethodPatch, "task/testTaskId", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "reading task modification options from JSON request body")
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

			req, err := http.NewRequest(http.MethodPatch, "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "reading task modification options from JSON request body")
		})
		Convey("then should error when fields not set", func() {
			badBod := &struct {
				Activated *string
			}{}
			res, err := json.Marshal(badBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest(http.MethodPatch, "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "must set activated or priority")
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

			req, err := http.NewRequest(http.MethodPatch, "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
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
	Convey("With a task in the DB and a Connector", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, serviceModel.VersionCollection, build.Collection))
		version := serviceModel.Version{
			Id: "v1",
		}
		build := build.Build{
			Id: "b1",
		}
		testTask := task.Task{
			Id:        "testTaskId",
			Version:   "v1",
			BuildId:   "b1",
			Activated: false,
			Priority:  10,
		}
		So(testTask.Insert(t.Context()), ShouldBeNil)
		So(version.Insert(t.Context()), ShouldBeNil)
		So(build.Insert(t.Context()), ShouldBeNil)
		ctx := context.Background()
		Convey("then setting priority should change it's priority", func() {
			act := true
			var prio int64 = 100

			tep := &taskExecutionPatchHandler{
				Activated: &act,
				Priority:  &prio,
				task: &task.Task{
					Id: "testTaskId",
				},
				user: &user.DBUser{
					Id: "testUser",
				},
			}
			res := tep.Run(ctx)
			So(res.Status(), ShouldEqual, http.StatusOK)
			resTask, ok := res.Data().(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Priority, ShouldEqual, int64(100))
			So(resTask.Activated, ShouldBeTrue)
			So(utility.FromStringPtr(resTask.ActivatedBy), ShouldEqual, "testUser")
		})
	})
}

func TestTaskResetPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		trh := &taskRestartHandler{}

		testTask := task.Task{
			Id:           "testTaskId",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: time.Now(),
			BuildId:      "b0",
			Version:      "v1",
			Status:       evergreen.TaskSucceeded,
			Priority:     0,
		}

		projCtx := serviceModel.Context{
			Task: &testTask,
		}
		u := user.DBUser{
			Id: "testUser",
		}
		ctx := context.Background()

		Convey("should error on empty project", func() {
			req, err := http.NewRequest(http.MethodPost, "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := "project not found"
			So(err.Error(), ShouldContainSubstring, expectedErr)
		})
		Convey("then should error on empty task", func() {
			projCtx.Task = nil
			req, err := http.NewRequest(http.MethodPost, "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message:    "task not found",
				StatusCode: http.StatusNotFound,
			}

			So(err, ShouldResemble, expectedErr)
		})

		projCtx.ProjectRef = &serviceModel.ProjectRef{
			Id:         "project_1",
			Identifier: "project_identifier",
		}

		failedOnlyTest := func(failedOnly bool) {
			projCtx.Task = &testTask
			json := []byte(`{"failed_only": ` + strconv.FormatBool(failedOnly) + `}`)
			buf := bytes.NewBuffer(json)
			req, err := http.NewRequest(http.MethodPost, "task/testTaskId/restart", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldBeNil)
			So(trh.FailedOnly, ShouldEqual, failedOnly)
		}

		Convey("should register true valued failedOnly parameter", func() {
			failedOnlyTest(true)
		})

		Convey("should register false valued failedOnly parameter", func() {
			failedOnlyTest(false)
		})

		Convey("should have default false failedOnly with empty body", func() {
			projCtx.Task = &testTask
			req, err := http.NewRequest(http.MethodPost, "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldBeNil)
			So(trh.FailedOnly, ShouldEqual, false)
		})
	})
}

func TestTaskGetHandler(t *testing.T) {
	Convey("With test server with a handler and mock data", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection))
		rm := makeGetTaskRoute("https://parsley.net/yee", "https://example.net/test")

		Convey("and task is in the service context", func() {
			newTask := task.Task{
				Id:        "testTaskId",
				Project:   "testProject",
				Execution: 1,
			}
			oldTask := task.Task{
				Id:        "testTaskId_0",
				OldTaskId: "testTaskId",
			}
			So(db.Insert(t.Context(), task.Collection, newTask), ShouldBeNil)
			So(db.Insert(t.Context(), task.OldCollection, oldTask), ShouldBeNil)

			app := gimlet.NewApp()
			app.SetPrefix("rest")
			app.AddRoute("/tasks/{task_id}").Version(2).Get().RouteHandler(rm)
			So(app.Resolve(), ShouldBeNil)
			r, err := app.Router()
			So(err, ShouldBeNil)

			Convey("a request with a user should then return no error and a task should"+
				" should be returned", func() {
				req, err := http.NewRequest(http.MethodGet, "/rest/v2/tasks/testTaskId", nil)
				So(err, ShouldBeNil)

				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				So(rr.Code, ShouldEqual, http.StatusOK)

				res := model.APITask{}
				err = json.Unmarshal(rr.Body.Bytes(), &res)
				So(err, ShouldBeNil)
				So(utility.FromStringPtr(res.Id), ShouldEqual, "testTaskId")
				So(utility.FromStringPtr(res.ProjectId), ShouldEqual, "testProject")
				So(len(res.PreviousExecutions), ShouldEqual, 0)
			})
			Convey("and old tasks are available", func() {
				Convey("a test that requests old executions should receive them", func() {
					req, err := http.NewRequest(http.MethodGet, "/rest/v2/tasks/testTaskId?fetch_all_executions=", nil)
					So(err, ShouldBeNil)

					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)
					So(rr.Code, ShouldEqual, http.StatusOK)

					res := model.APITask{}
					err = json.Unmarshal(rr.Body.Bytes(), &res)
					So(err, ShouldBeNil)
					So(len(res.PreviousExecutions), ShouldEqual, 1)
				})
				Convey("a test that doesn't request old executions should not receive them", func() {
					req, err := http.NewRequest(http.MethodGet, "/rest/v2/tasks/testTaskId", nil)
					So(err, ShouldBeNil)

					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)
					So(rr.Code, ShouldEqual, http.StatusOK)

					res := model.APITask{}
					err = json.Unmarshal(rr.Body.Bytes(), &res)
					So(err, ShouldBeNil)
					So(len(res.PreviousExecutions), ShouldEqual, 0)
				})

			})
		})
	})
}

func TestTaskResetExecute(t *testing.T) {
	Convey("With a task returned by the Connector", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, serviceModel.VersionCollection, build.Collection))
		timeNow := time.Now()
		testTask := task.Task{
			Id:           "testTaskId",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: timeNow,
			BuildId:      "b0",
			Version:      "v1",
			Status:       evergreen.TaskSucceeded,
		}
		So(testTask.Insert(t.Context()), ShouldBeNil)

		testTask2 := task.Task{
			Id:           "testTaskId2",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: timeNow,
			BuildId:      "b0",
			Version:      "v1",
			Status:       evergreen.TaskFailed,
		}
		So(testTask2.Insert(t.Context()), ShouldBeNil)

		testTask3 := task.Task{
			Id:           "testTaskId3",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: timeNow,
			BuildId:      "b0",
			Version:      "v1",
			Status:       evergreen.TaskSucceeded,
		}
		So(testTask3.Insert(t.Context()), ShouldBeNil)

		displayTask := &task.Task{
			Id:             "displayTask",
			DisplayName:    "displayTask",
			BuildId:        "b0",
			Version:        "v1",
			Activated:      false,
			DisplayOnly:    true,
			ExecutionTasks: []string{testTask2.Id, testTask3.Id},
			Status:         evergreen.TaskFailed,
			DispatchTime:   time.Now(),
		}
		So(displayTask.Insert(t.Context()), ShouldBeNil)

		v := &serviceModel.Version{Id: "v1"}
		So(v.Insert(t.Context()), ShouldBeNil)
		b := build.Build{Id: "b0", Version: "v1", Activated: true}
		So(b.Insert(t.Context()), ShouldBeNil)

		ctx := context.Background()
		Convey("and an error from the service function", func() {
			testTask4 := task.Task{
				Id:           "testTaskId4",
				Activated:    false,
				Secret:       "initialSecret",
				DispatchTime: timeNow,
				BuildId:      "b0",
				Version:      "v1",
				Status:       evergreen.TaskStarted,
			}
			So(testTask4.Insert(t.Context()), ShouldBeNil)
			trh := &taskRestartHandler{
				taskId:   "testTaskId4",
				username: "testUser",
			}
			resp := trh.Run(ctx)
			So(resp.Status(), ShouldNotEqual, http.StatusOK)
			apiErr, ok := resp.Data().(gimlet.ErrorResponse)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusBadRequest)

		})

		Convey("calling TaskRestartHandler should reset the task", func() {
			trh := &taskRestartHandler{
				taskId:   "testTaskId",
				username: "testUser",
			}

			res := trh.Run(ctx)
			So(res.Status(), ShouldEqual, http.StatusOK)
			resTask, ok := res.Data().(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Activated, ShouldBeTrue)
			So(resTask.DispatchTime, ShouldBeNil)
			dbTask, err := task.FindOneId(ctx, "testTaskId")
			So(err, ShouldBeNil)
			So(dbTask.Secret, ShouldNotResemble, "initialSecret")
		})

		Convey("calling TaskRestartHandler should reset the task with failedonly", func() {
			trh := &taskRestartHandler{
				taskId:     "displayTask",
				username:   "testUser",
				FailedOnly: true,
			}

			res := trh.Run(ctx)
			So(res.Status(), ShouldEqual, http.StatusOK)
			resTask, ok := res.Data().(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Activated, ShouldBeTrue)
			So(resTask.DispatchTime, ShouldBeNil)
			dbTask2, err := task.FindOneId(ctx, "testTaskId2")
			So(err, ShouldBeNil)
			So(dbTask2.Secret, ShouldNotResemble, "initialSecret")
			So(dbTask2.Status, ShouldEqual, evergreen.TaskUndispatched)
			dbTask3, err := task.FindOneId(ctx, "testTaskId3")
			So(err, ShouldBeNil)
			So(dbTask3.Secret, ShouldResemble, "initialSecret")
			So(dbTask3.Status, ShouldEqual, evergreen.TaskSucceeded)
		})
	})

}

func TestParentTaskInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(task.Collection))
	buildID := "test"
	dtID := "displayTask"
	displayTask := task.Task{
		Id:             dtID,
		BuildId:        buildID,
		DisplayOnly:    true,
		ExecutionTasks: []string{"execTask0", "execTask1"},
	}
	execTask0 := task.Task{
		Id:      "execTask0",
		BuildId: buildID,
	}
	execTask1 := task.Task{
		Id:      "execTask1",
		BuildId: buildID,
	}
	randomTask := task.Task{
		Id:      "randomTask",
		BuildId: buildID,
	}

	assert.NoError(t, displayTask.Insert(t.Context()))
	assert.NoError(t, execTask0.Insert(t.Context()))
	assert.NoError(t, execTask1.Insert(t.Context()))
	assert.NoError(t, randomTask.Insert(t.Context()))
	tbh := &tasksByBuildHandler{
		limit: 100,
		url:   "http://evergreen.example.net",
	}
	route := "/rest/v2/builds/test/tasks?fetch_all_executions=false&fetch_parent_ids=true&start_at=execTask0"
	r, err := http.NewRequest(http.MethodGet, route, nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"build_id": "test"})

	assert.NoError(t, tbh.Parse(ctx, r))
	assert.False(t, tbh.fetchAllExecutions)
	assert.True(t, tbh.fetchParentIds)

	resp := tbh.Run(ctx)
	data, ok := resp.Data().([]any)
	assert.True(t, ok)
	assert.Len(t, data, 3)

	expectedParentIDs := []string{dtID, dtID, ""}
	for i := range data {
		task, ok := data[i].(*model.APITask)
		require.True(t, ok)
		assert.Equal(t, expectedParentIDs[i], task.ParentTaskId)
	}
}

func TestOptionsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(task.Collection))

	route := "/rest/v2/tasks/test/restart"
	_, err := http.NewRequest("OPTIONS", route, nil)
	assert.NoError(t, err)

	tbh := &optionsHandler{}
	resp := tbh.Run(ctx)
	data := resp.Data()
	assert.Equal(t, struct{}{}, data)
	assert.Equal(t, http.StatusOK, resp.Status())

}

func validatePaginatedResponse(t *testing.T, h gimlet.RouteHandler, expected []any, pages *gimlet.ResponsePages) {
	require.NotNil(t, h)
	require.NotNil(t, pages)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := h.Run(ctx)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.Status())
	rpg := resp.Pages()
	require.NotNil(t, rpg)

	assert.True(t, pages.Next != nil || pages.Prev != nil)
	assert.True(t, rpg.Next != nil || rpg.Prev != nil)
	if pages.Next != nil {
		assert.Equal(t, pages.Next.BaseURL, rpg.Next.BaseURL)
		assert.Equal(t, pages.Next.Key, rpg.Next.Key)
		assert.Equal(t, pages.Next.Limit, rpg.Next.Limit)
		assert.Equal(t, pages.Next.Relation, rpg.Next.Relation)
	} else if pages.Prev != nil {
		assert.Equal(t, pages.Prev.Key, rpg.Prev.Key)
		assert.Equal(t, pages.Prev.Limit, rpg.Prev.Limit)
		assert.Equal(t, pages.Prev.Relation, rpg.Prev.Relation)
		assert.Equal(t, pages.Prev.BaseURL, rpg.Prev.BaseURL)
	}

	assert.EqualValues(t, pages, rpg)

	data, ok := resp.Data().([]any)
	assert.True(t, ok)

	if !assert.Equal(t, len(expected), len(data)) {
		return
	}

	for idx := range expected {
		assert.Equal(t, expected[idx], data[idx])
	}
}
