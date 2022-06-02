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
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
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
				So(nextHost.Insert(), ShouldBeNil)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				hostToStartAt := 100
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:      utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL: utility.ToStringPtr(""),
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
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:      utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL: utility.ToStringPtr(""),
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
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:      utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL: utility.ToStringPtr(""),
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
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					nextModelHost := &model.APIHost{
						Id:      utility.ToStringPtr(fmt.Sprintf("%dhost%d", prefix, i)),
						HostURL: utility.ToStringPtr(""),
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
		assert.NoError(t, p.Insert())
		Convey("and there are tasks to be found", func() {
			cachedTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextTask := task.Task{
					Id:       fmt.Sprintf("%dtask_%d", prefix, i),
					Revision: commit,
					Project:  projectId,
				}
				cachedTasks = append(cachedTasks, nextTask)
			}
			for _, cachedTask := range cachedTasks {
				err := db.Insert(task.Collection, cachedTask)
				So(err, ShouldBeNil)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("%dtask_%d", prefix, i),
						Revision: commit,
						Project:  projectId,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("%dtask_%d", prefix, i),
						Revision: commit,
						Project:  projectId,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
				}

				validatePaginatedResponse(t, handler, expectedTasks, expectedPages)
			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("%dtask_%d", prefix, i),
						Revision: commit,
						Project:  projectId,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("%dtask_%d", prefix, i),
						Revision: commit,
						Project:  projectId,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceTask, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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

func TestTaskByBuildPaginator(t *testing.T) {
	numTasks := 300
	Convey("When paginating with a Connector", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection))
		Convey("and there are tasks to be found", func() {
			cachedTasks := []task.Task{}
			cachedOldTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				prefix := int(math.Log10(float64(i)))
				if i == 0 {
					prefix = 0
				}
				nextTask := task.Task{
					Id: fmt.Sprintf("%dbuild%d", prefix, i),
				}
				So(db.Insert(task.Collection, nextTask), ShouldBeNil)
				cachedTasks = append(cachedTasks, nextTask)
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
				So(db.Insert(task.OldCollection, nextTask), ShouldBeNil)
				cachedOldTasks = append(cachedOldTasks, nextTask)
			}
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceModel, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
					limit: limit,
					key:   fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:   "http://evergreen.example.net",
				}

				// SPARTA
				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				taskToStartAt := 150
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceModel, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
					limit: limit,
					key:   fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:   "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceModel, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
					limit: limit,
					key:   fmt.Sprintf("%dbuild%d", prefix, taskToStartAt),
					url:   "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				taskToStartAt := 0
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					prefix := int(math.Log10(float64(i)))
					if i == 0 {
						prefix = 0
					}
					serviceModel := &task.Task{
						Id: fmt.Sprintf("%dbuild%d", prefix, i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromArgs(serviceModel, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
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
					limit: limit,
					key:   fmt.Sprintf("%dbuild%d", 0, taskToStartAt),
					url:   "http://evergreen.example.net",
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("pagination with tasks with previous executions", func() {
				expectedTasks := []model.Model{}
				serviceModel := &task.Task{
					Id: "0build0",
				}
				nextModelTask := &model.APITask{}
				err := nextModelTask.BuildFromArgs(serviceModel, &model.APITaskArgs{LogURL: "http://evergreen.example.net", IncludeProjectIdentifier: true})
				So(err, ShouldBeNil)
				err = nextModelTask.BuildPreviousExecutions(cachedOldTasks, "http://evergreen.example.net")
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
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})
		})
	})
}

func TestTestPaginator(t *testing.T) {
	numTests := 300
	Convey("When paginating with a Connector", t, func() {
		serviceContext := data.MockGitHubConnector{
			URL: "http://evergreen.example.net",
		}
		Convey("and there are tasks with tests to be found", func() {
			cachedTests := []testresult.TestResult{}
			for i := 0; i < numTests; i++ {
				status := "pass"
				if i%2 == 0 {
					status = "fail"
				}
				nextTest := testresult.TestResult{
					ID:     mgobson.ObjectId(fmt.Sprintf("object_id_%d_", i)),
					TaskID: "myTask",
					Status: status,
				}
				cachedTests = append(cachedTests, nextTest)
			}
			myTask := task.Task{
				Id: "myTask",
			}
			serviceContext.CachedTests = cachedTests
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				testToStartAt := 100
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					nextModelTest := &model.APITest{}
					_ = nextModelTest.BuildFromService(&cachedTests[i])
					_ = nextModelTest.BuildFromService("")
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				handler := &testGetHandler{
					limit: limit,
					key:   fmt.Sprintf("object_id_%d_", testToStartAt),
					sc:    &serviceContext,
					task:  &myTask,
				}

				validatePaginatedResponse(t, handler, expectedTests, expectedPages)
			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				testToStartAt := 150
				limit := 50
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					nextModelTest := &model.APITest{}
					_ = nextModelTest.BuildFromService(&cachedTests[i])
					_ = nextModelTest.BuildFromService("")
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("object_id_%d_", testToStartAt+50),
						Limit:           50,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				handler := &testGetHandler{
					limit: 50,
					key:   fmt.Sprintf("object_id_%d_", testToStartAt),
					sc:    &serviceContext,
					task:  &myTask,
				}

				validatePaginatedResponse(t, handler, expectedTests, expectedPages)
			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				testToStartAt := 50
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					nextModelTest := &model.APITest{}
					_ = nextModelTest.BuildFromService(&cachedTests[i])
					_ = nextModelTest.BuildFromService("")
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				handler := &testGetHandler{
					key:   fmt.Sprintf("object_id_%d_", testToStartAt),
					limit: limit,
					sc:    &serviceContext,
					task:  &myTask,
				}

				validatePaginatedResponse(t, handler, expectedTests, expectedPages)
			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				testToStartAt := 0
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					nextModelTest := &model.APITest{}
					_ = nextModelTest.BuildFromService(&cachedTests[i])
					_ = nextModelTest.BuildFromService("")
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         "http://evergreen.example.net",
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				handler := &testGetHandler{
					key:   fmt.Sprintf("object_id_%d_", testToStartAt),
					sc:    &serviceContext,
					limit: limit,
					task:  &myTask,
				}

				validatePaginatedResponse(t, handler, expectedTests, expectedPages)
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
			req, err := http.NewRequest("PATCH", "task/testTaskId", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
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
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
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
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
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
		So(testTask.Insert(), ShouldBeNil)
		So(version.Insert(), ShouldBeNil)
		So(build.Insert(), ShouldBeNil)
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

		Convey("should error on empty project", func() {
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := "Project not found"
			So(err.Error(), ShouldContainSubstring, expectedErr)
		})
		Convey("then should error on empty task", func() {
			projCtx.Task = nil
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.Parse(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message:    "Task not found",
				StatusCode: http.StatusNotFound,
			}

			So(err, ShouldResemble, expectedErr)
		})
	})
}

func TestTaskGetHandler(t *testing.T) {
	Convey("With test server with a handler and mock data", t, func() {
		assert.NoError(t, db.ClearCollections(task.Collection, task.OldCollection))
		rm := makeGetTaskRoute("https://example.net/test")

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
			So(db.Insert(task.Collection, newTask), ShouldBeNil)
			So(db.Insert(task.OldCollection, oldTask), ShouldBeNil)

			app := gimlet.NewApp()
			app.SetPrefix("rest")
			app.AddRoute("/tasks/{task_id}").Version(2).Get().RouteHandler(rm)
			So(app.Resolve(), ShouldBeNil)
			r, err := app.Router()
			So(err, ShouldBeNil)

			Convey("a request with a user should then return no error and a task should"+
				" should be returned", func() {
				req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId", nil)
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
					req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId?fetch_all_executions=", nil)
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
					req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId", nil)
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
		So(testTask.Insert(), ShouldBeNil)
		v := &serviceModel.Version{Id: "v1"}
		So(v.Insert(), ShouldBeNil)
		b := build.Build{Id: "b0", Version: "v1", Activated: true}
		So(b.Insert(), ShouldBeNil)
		ctx := context.Background()
		Convey("and an error from the service function", func() {
			testTask2 := task.Task{
				Id:           "testTaskId2",
				Activated:    false,
				Secret:       "initialSecret",
				DispatchTime: timeNow,
				BuildId:      "b0",
				Version:      "v1",
				Status:       evergreen.TaskStarted,
			}
			So(testTask2.Insert(), ShouldBeNil)
			trh := &taskRestartHandler{
				taskId:   "testTaskId2",
				username: "testUser",
			}
			resp := trh.Run(ctx)
			So(resp.Status(), ShouldNotEqual, http.StatusOK)
			apiErr, ok := resp.Data().(gimlet.ErrorResponse)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusBadRequest)

		})

		Convey("calling TryReset should reset the task", func() {
			trh := &taskRestartHandler{
				taskId:   "testTaskId",
				username: "testUser",
			}

			res := trh.Run(ctx)
			So(res.Status(), ShouldEqual, http.StatusOK)
			resTask, ok := res.Data().(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Activated, ShouldBeTrue)
			So(resTask.DispatchTime, ShouldEqual, nil)
			dbTask, err := task.FindOneId("testTaskId")
			So(err, ShouldBeNil)
			So(dbTask.Secret, ShouldNotResemble, "initialSecret")
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

	assert.NoError(t, displayTask.Insert())
	assert.NoError(t, execTask0.Insert())
	assert.NoError(t, execTask1.Insert())
	assert.NoError(t, randomTask.Insert())
	tbh := &tasksByBuildHandler{
		limit: 100,
		url:   "http://evergreen.example.net",
	}
	route := "/rest/v2/builds/test/tasks?fetch_all_executions=false&fetch_parent_ids=true&start_at=execTask0"
	r, err := http.NewRequest("GET", route, nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"build_id": "test"})

	assert.NoError(t, tbh.Parse(ctx, r))
	assert.False(t, tbh.fetchAllExecutions)
	assert.True(t, tbh.fetchParentIds)

	resp := tbh.Run(ctx)
	data, ok := resp.Data().([]interface{})
	assert.True(t, ok)
	assert.Equal(t, 3, len(data))

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
	assert.Equal(t, data, struct{}{})
	assert.Equal(t, resp.Status(), http.StatusOK)

}

func validatePaginatedResponse(t *testing.T, h gimlet.RouteHandler, expected []model.Model, pages *gimlet.ResponsePages) {
	if !assert.NotNil(t, h) {
		return
	}
	if !assert.NotNil(t, pages) {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	rpg := resp.Pages()
	if !assert.NotNil(t, rpg) {
		return
	}

	assert.True(t, pages.Next != nil || pages.Prev != nil)
	assert.True(t, rpg.Next != nil || rpg.Prev != nil)
	if pages.Next != nil {
		assert.Equal(t, pages.Next.Key, rpg.Next.Key)
		assert.Equal(t, pages.Next.Limit, rpg.Next.Limit)
		assert.Equal(t, pages.Next.Relation, rpg.Next.Relation)
	} else if pages.Prev != nil {
		assert.Equal(t, pages.Prev.Key, rpg.Prev.Key)
		assert.Equal(t, pages.Prev.Limit, rpg.Prev.Limit)
		assert.Equal(t, pages.Prev.Relation, rpg.Prev.Relation)
	}

	assert.EqualValues(t, pages, rpg)

	data, ok := resp.Data().([]interface{})
	assert.True(t, ok)

	if !assert.Equal(t, len(expected), len(data)) {
		return
	}

	for idx := range expected {
		m, ok := data[idx].(model.Model)

		if assert.True(t, ok) {
			assert.Equal(t, expected[idx], m)
		}
	}
}
