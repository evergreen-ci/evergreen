package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchByIdSuite struct {
	objIds []string
	route  *patchByIdHandler
	suite.Suite
}

func TestPatchByIdSuite(t *testing.T) {
	suite.Run(t, new(PatchByIdSuite))
}

func (s *PatchByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection))
	s.objIds = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}

	patches := []patch.Patch{
		{Id: patch.NewId(s.objIds[0])},
		{Id: patch.NewId(s.objIds[1])},
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
}

func (s *PatchByIdSuite) SetupTest() {
	s.route = makeFetchPatchByID().(*patchByIdHandler)
}

func (s *PatchByIdSuite) TestFindById() {
	s.route.patchId = s.objIds[0]
	res := s.route.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status(), "%+v", res.Data())

	p, ok := res.Data().(*model.APIPatch)
	s.True(ok)
	s.Equal(utility.ToStringPtr(s.objIds[0]), p.Id)
}
func (s *PatchByIdSuite) TestFindByIdFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.objIds {
		s.NotEqual(new_id, i)
	}

	s.route.patchId = new_id.Hex()
	res := s.route.Run(context.TODO())
	s.Equal(http.StatusNotFound, res.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type PatchesByProjectSuite struct {
	now   time.Time
	route *patchesByProjectHandler
	suite.Suite
}

func TestPatchesByProjectSuite(t *testing.T) {
	suite.Run(t, new(PatchesByProjectSuite))
}

func (s *PatchesByProjectSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, serviceModel.ProjectRefCollection))
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)
	proj1 := "project1"
	proj1Identifier := "project_one"
	proj2 := "project2"
	proj2Identifier := "project_two"
	proj3 := "project3"
	proj3Identifier := "project_three"
	nowPlus2 := s.now.Add(time.Second * 2)
	nowPlus4 := s.now.Add(time.Second * 4)
	nowPlus6 := s.now.Add(time.Second * 6)
	nowPlus8 := s.now.Add(time.Second * 8)
	nowPlus10 := s.now.Add(time.Second * 10)
	projects := []serviceModel.ProjectRef{
		{
			Id:         proj1,
			Identifier: proj1Identifier,
		},
		{
			Id:         proj2,
			Identifier: proj2Identifier,
		},
		{
			Id:         proj3,
			Identifier: proj3Identifier,
		},
	}
	patches := []patch.Patch{
		{Project: proj1, CreateTime: s.now},
		{Project: proj2, CreateTime: nowPlus2},
		{Project: proj1, CreateTime: nowPlus4},
		{Project: proj1, CreateTime: nowPlus6},
		{Project: proj2, CreateTime: nowPlus8},
		{Project: proj1, CreateTime: nowPlus10},
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
	for _, proj := range projects {
		s.NoError(proj.Insert())
	}
}

func (s *PatchesByProjectSuite) SetupTest() {
	s.route = makePatchesByProjectRoute("https://evergreen.example.net").(*patchesByProjectHandler)
}

func (s *PatchesByProjectSuite) TestPaginatorShouldSucceedIfNoResults() {
	s.route.projectId = "project3"
	s.route.key = s.now
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status(), "%+v", resp.Data())
}

func (s *PatchesByProjectSuite) TestPaginatorShouldFailIfNoProject() {
	s.route.projectId = "zzz"
	s.route.key = s.now
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusInternalServerError, resp.Status())
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	s.route.projectId = "project1"
	s.route.key = s.now.Add(time.Second * 7)
	s.route.limit = 2

	resp := s.route.Run(context.Background())
	s.NotNil(resp)

	payload := resp.Data().([]interface{})
	s.NotNil(payload)

	s.Len(payload, 2)
	s.Equal(s.now.Add(time.Second*6), *(payload[0]).(model.APIPatch).CreateTime)
	s.Equal(s.now.Add(time.Second*4), *(payload[1]).(model.APIPatch).CreateTime)

	pages := resp.Pages()
	s.NotNil(pages)
	s.Nil(pages.Prev)
	s.NotNil(pages.Next)

	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pages.Next.Key)
}

func (s *PatchesByProjectSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	s.route.projectId = "project2"
	s.route.key = s.now.Add(time.Hour)
	s.route.limit = 100

	resp := s.route.Run(context.Background())
	s.Len(resp.Data().([]interface{}), 2)

	s.Nil(resp.Pages())
}

func (s *PatchesByProjectSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			req, err := http.NewRequest(http.MethodGet, "https://example.net/foo/?limit=10&start_at="+i, nil)
			s.Require().NoError(err)
			err = s.route.Parse(context.Background(), req)
			s.Error(err)
		}
	}
}

func (s *PatchesByProjectSuite) TestEmptyTimeShouldSetNow() {
	req, err := http.NewRequest(http.MethodGet, "https://example.net/foo/?limit=10", nil)
	s.Require().NoError(err)
	s.NoError(s.route.Parse(context.Background(), req))
	s.InDelta(time.Now().UnixNano(), s.route.key.UnixNano(), float64(time.Second))
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort patch route

type PatchAbortSuite struct {
	objIds []string

	suite.Suite
}

func TestPatchAbortSuite(t *testing.T) {
	suite.Run(t, new(PatchAbortSuite))
}

func (s *PatchAbortSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, task.Collection, serviceModel.VersionCollection, build.Collection))
	s.objIds = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456", "aabbccddeeff001122334457"}
	version1 := "version1"
	version2 := "version2"

	patches := []patch.Patch{
		{Id: patch.NewId(s.objIds[0]), Version: version1, Activated: true},
		{Id: patch.NewId(s.objIds[1]), Version: version2, Activated: true},
		{Id: patch.NewId(s.objIds[2]), Activated: true},
	}
	versions := []*serviceModel.Version{
		{Id: version1},
		{Id: version2},
	}

	tasks := []*task.Task{
		{Id: "task1", Version: "version1", Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task2", Version: "version2", Aborted: false, Status: evergreen.TaskDispatched},
	}

	builds := []*build.Build{
		{Id: "build1"},
	}

	for _, item := range versions {
		s.NoError(item.Insert())
	}
	for _, item := range tasks {
		s.NoError(item.Insert())
	}
	for _, item := range builds {
		s.NoError(item.Insert())
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
}

func (s *PatchAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeAbortPatch().(*patchAbortHandler)
	rm.patchId = s.objIds[0]
	res := rm.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	task1, err := task.FindOneId("task1")
	s.NoError(err)
	s.True(task1.Aborted)
	task2, err := task.FindOneId("task2")
	s.NoError(err)
	s.False(task2.Aborted)
	p, ok := (res.Data()).(*model.APIPatch)
	s.True(ok)
	s.Equal(utility.ToStringPtr(s.objIds[0]), p.Id)

	rm.patchId = s.objIds[1]
	res = rm.Run(ctx)
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	task1, err = task.FindOneId("task1")
	s.NoError(err)
	s.True(task1.Aborted)
	task2, err = task.FindOneId("task2")
	s.NoError(err)
	s.True(task2.Aborted)
	p, ok = (res.Data()).(*model.APIPatch)
	s.True(ok)
	s.Equal(utility.ToStringPtr(s.objIds[1]), p.Id)

	rm.patchId = s.objIds[2]
	res = rm.Run(ctx)

	s.NotEqual(http.StatusOK, res.Status())
}

func (s *PatchAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeAbortPatch().(*patchAbortHandler)
	new_id := mgobson.NewObjectId()
	for _, i := range s.objIds {
		s.NotEqual(new_id, i)
	}

	rm.patchId = new_id.Hex()
	res := rm.Run(ctx)
	s.NotEqual(http.StatusOK, res.Status())
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change patch status route

type PatchesChangeStatusSuite struct {
	objIds []string

	suite.Suite
}

func TestPatchesChangeStatusSuite(t *testing.T) {
	suite.Run(t, new(PatchesChangeStatusSuite))
}

func (s *PatchesChangeStatusSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, serviceModel.ProjectRefCollection, build.Collection, task.Collection, serviceModel.VersionCollection))
	s.objIds = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}

	patches := []patch.Patch{
		{Id: patch.NewId(s.objIds[0]), Version: "v1", Project: "proj", PatchNumber: 7},
		{Id: patch.NewId(s.objIds[1]), Version: "v1", Project: "proj", PatchNumber: 0},
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
	v := &serviceModel.Version{Id: "v1"}
	s.NoError(v.Insert())
	b := build.Build{Id: "b0", Version: "v1", Activated: true}
	s.NoError(b.Insert())
	tasks := []task.Task{
		{Id: "t0", BuildId: "b0", Activated: true, Status: evergreen.TaskUndispatched},
		{Id: "t1", BuildId: "b0", Activated: true, Status: evergreen.TaskSucceeded},
	}
	for _, t := range tasks {
		s.NoError(t.Insert())
	}
}

func (s *PatchesChangeStatusSuite) TestChangeStatus() {
	p := serviceModel.ProjectRef{
		Id:         "proj",
		Identifier: "proj",
	}
	s.NoError(p.Insert())
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})
	env := testutil.NewEnvironment(ctx, s.T())
	rm := makeChangePatchStatus(env).(*patchChangeStatusHandler)

	rm.patchId = s.objIds[0]
	var tmp_true = true
	rm.Activated = &tmp_true
	var tmp_seven = int64(7)
	rm.Priority = &tmp_seven
	res := rm.Run(ctx)
	s.NotNil(res)
	p1, err := data.FindPatchById(s.objIds[0])
	s.NoError(err)
	p2, err := data.FindPatchById(s.objIds[1])
	s.NoError(err)
	s.Equal(7, p1.PatchNumber)
	s.Equal(0, p2.PatchNumber)
	s.True(p1.Activated)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for restart patch by id route

type PatchRestartSuite struct {
	objIds []string

	suite.Suite
}

func TestPatchRestartSuite(t *testing.T) {
	suite.Run(t, new(PatchRestartSuite))
}

func (s *PatchRestartSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, serviceModel.VersionCollection, task.Collection))
	s.objIds = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}
	version1 := "version1"

	versions := []serviceModel.Version{
		{
			Id: s.objIds[0],
		},
		{
			Id: s.objIds[1],
		},
	}
	tasks := []task.Task{
		{
			Id:      "task1",
			Version: s.objIds[0],
		},
	}
	patches := []patch.Patch{
		{Id: patch.NewId(s.objIds[0]), Version: version1},
		{Id: patch.NewId(s.objIds[1])},
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
	for _, v := range versions {
		s.NoError(v.Insert())
	}
	for _, t := range tasks {
		s.NoError(t.Insert())
	}
}

func (s *PatchRestartSuite) TestRestart() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user1"})

	rm := makeRestartPatch().(*patchRestartHandler)
	rm.patchId = s.objIds[0]
	res := rm.Run(ctx)
	s.NotNil(res)

	s.Equal(http.StatusOK, res.Status())
	restartedPatch, err := data.FindPatchById(s.objIds[0])
	s.NoError(err)
	s.Equal(evergreen.PatchVersionRequester, *restartedPatch.Requester)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patches for current user

type PatchesByUserSuite struct {
	now   time.Time
	route *patchesByUserHandler

	suite.Suite
}

func TestPatchesByUserSuite(t *testing.T) {
	s := new(PatchesByUserSuite)
	suite.Run(t, s)
}

func (s *PatchesByUserSuite) SetupTest() {
	s.NoError(db.ClearCollections(patch.Collection))
	s.route = &patchesByUserHandler{
		url: "http://evergreen.example.net/",
	}
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)
	user1 := "user1"
	user2 := "user2"
	nowPlus2 := s.now.Add(time.Second * 2)
	nowPlus4 := s.now.Add(time.Second * 4)
	nowPlus6 := s.now.Add(time.Second * 6)
	nowPlus8 := s.now.Add(time.Second * 8)
	nowPlus10 := s.now.Add(time.Second * 10)
	patches := []patch.Patch{
		{Author: user1, CreateTime: s.now},
		{Author: user2, CreateTime: nowPlus2},
		{Author: user1, CreateTime: nowPlus4},
		{Author: user1, CreateTime: nowPlus6},
		{Author: user2, CreateTime: nowPlus8},
		{Author: user1, CreateTime: nowPlus10},
	}
	for _, p := range patches {
		s.NoError(p.Insert())
	}
}

func (s *PatchesByUserSuite) TestPaginatorShouldSucceedIfNoResults() {
	s.route.user = "zzz"
	s.route.key = s.now
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status(), "%+v", resp.Data())
}

func (s *PatchesByUserSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	s.route.user = "user1"
	s.route.key = s.now.Add(time.Second * 7)
	s.route.limit = 2

	resp := s.route.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})

	s.Len(payload, 2)
	s.Equal(s.now.Add(time.Second*6), *(payload[0]).(model.APIPatch).CreateTime)
	s.Equal(s.now.Add(time.Second*4), *(payload[1]).(model.APIPatch).CreateTime)

	pageData := resp.Pages()

	s.Nil(pageData.Prev)
	s.NotNil(pageData.Next)

	nextTime := s.now.Format(model.APITimeFormat)
	s.Equal(nextTime, pageData.Next.Key)
}

func (s *PatchesByUserSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	s.route.user = "user2"
	s.route.key = s.now.Add(time.Hour)
	s.route.limit = 100

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	s.Len(resp.Data().([]interface{}), 2)

	s.Nil(resp.Pages())
}

func (s *PatchesByUserSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			req, err := http.NewRequest(http.MethodGet, "https://example.net/foo/?limit=10&start_at="+i, nil)
			s.Require().NoError(err)
			err = s.route.Parse(context.Background(), req)
			s.Require().Error(err)
			s.Contains(err.Error(), i)
		}
	}
}

func (s *PatchesByUserSuite) TestEmptyTimeShouldSetNow() {
	req, err := http.NewRequest(http.MethodGet, "https://example.net/foo/?limit=10", nil)
	s.Require().NoError(err)
	s.NoError(s.route.Parse(context.Background(), req))
	s.InDelta(time.Now().UnixNano(), s.route.key.UnixNano(), float64(time.Second))
}

func TestPatchRawModulesHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(patch.Collection))
	require.NoError(t, db.ClearGridCollections(patch.GridFSPrefix))
	patchString := `main diff`
	require.NoError(t, db.WriteGridFile(patch.GridFSPrefix, "testPatch", strings.NewReader(patchString)))
	patchString = `module1 diff`
	require.NoError(t, db.WriteGridFile(patch.GridFSPrefix, "module1Patch", strings.NewReader(patchString)))
	patchString = `module2 diff`
	require.NoError(t, db.WriteGridFile(patch.GridFSPrefix, "module2Patch", strings.NewReader(patchString)))
	patchId := mgobson.NewObjectId()
	patchToInsert := patch.Patch{
		Id: patchId,
		Patches: []patch.ModulePatch{
			{
				ModuleName: "",
				PatchSet: patch.PatchSet{
					PatchFileId:    "testPatch",
					CommitMessages: []string{"Commit 1", "Commit 2"},
				},
			},
			{
				ModuleName: "module1",
				PatchSet: patch.PatchSet{
					PatchFileId:    "module1Patch",
					CommitMessages: []string{"Commit 1", "Commit 2"},
				},
			},
			{
				ModuleName: "module2",
				PatchSet: patch.PatchSet{
					PatchFileId:    "module2Patch",
					CommitMessages: []string{"Commit 1", "Commit 2"},
				},
			},
		},
	}
	assert.NoError(t, patchToInsert.Insert())

	route := &moduleRawHandler{
		patchID: patchId.Hex(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response := route.Run(ctx)

	rawModulesResponse, ok := response.Data().(*restmodel.APIRawPatch)
	require.True(t, ok)

	rp := rawModulesResponse.Patch
	modules := rawModulesResponse.RawModules
	assert.Equal(t, rp.Diff, "main diff")
	assert.Equal(t, len(modules), 2)

	assert.Equal(t, modules[0].Name, "module1")
	assert.Equal(t, modules[0].Diff, "module1 diff")

	assert.Equal(t, modules[1].Name, "module2")
	assert.Equal(t, modules[1].Diff, "module2 diff")
}

func TestPatchRawHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(patch.Collection))
	require.NoError(t, db.ClearGridCollections(patch.GridFSPrefix))
	patchString := `main diff`
	require.NoError(t, db.WriteGridFile(patch.GridFSPrefix, "testPatch", strings.NewReader(patchString)))
	patchString = `module1 diff`
	require.NoError(t, db.WriteGridFile(patch.GridFSPrefix, "module1Patch", strings.NewReader(patchString)))
	patchId := "aabbccddeeff001122334455"
	patchToInsert := patch.Patch{
		Id: patch.NewId(patchId),
		Patches: []patch.ModulePatch{
			{
				ModuleName: "main",
				PatchSet: patch.PatchSet{
					PatchFileId:    "testPatch",
					CommitMessages: []string{"Commit 1", "Commit 2"},
				},
			},
			{
				ModuleName: "module1",
				PatchSet: patch.PatchSet{
					PatchFileId:    "module1Patch",
					CommitMessages: []string{"Commit 1", "Commit 2"},
				},
			},
		},
	}
	assert.NoError(t, patchToInsert.Insert())

	route := &patchRawHandler{
		patchID:    patchId,
		moduleName: "main",
	}

	response := route.Run(context.Background())
	assert.Equal(t, http.StatusOK, response.Status())
	assert.Equal(t, "main diff", response.Data())

	route.moduleName = "module1"
	response = route.Run(context.Background())
	assert.Equal(t, http.StatusOK, response.Status())
	assert.Equal(t, "module1 diff", response.Data())
}

func TestSchedulePatchRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	const config = `
functions:
  "fetch source" :
    - command: git.get_project
      params:
        directory: src
    - command: shell.exec
      params:
        working_dir: src
        script: |
          echo "this is a 2nd command in the function!"
          ls
  "debug":
    command: shell.exec
    params:
      script: |
        echo "i am a debug function."
  "run a task that fails" :
    command: shell.exec
    params:
      working_dir: src
      script: |
        echo "this is a function with only a single command to run!"
        ./run.py results fail
  "run a task that passes" :
    command: shell.exec
    params:
      working_dir: src
      script: |
        ./run.py results pass
  "run a function with an arg":
    command: shell.exec
    params:
      working_dir: src
      script: |
        echo "I was called with ${foobar}"
pre:
  command: shell.exec
  params:
    script: |
      rm -rf src || true
      echo "pre-task run. JUST ONE COMMAND"
post:
  - command: shell.exec
    params:
      script: |
        echo "post-task run."
        true
  - command: attach.results
    params:
      file_location: src/results.json

tasks:
- name: compile
  depends_on: []
  commands:
    - func: "fetch source"
    - func: "run a task that passes" 
    - func: "run a function with an arg"
      vars:
        foobar: "TESTING: ONE"
    - func: "run a function with an arg"
      vars:
        foobar: "TESTING: TWO"

- name: passing_test 
  depends_on: 
  - name: compile
  commands:
    - func: "fetch source"
    - func: "run a task that passes"

- name: failing_test 
  depends_on: 
  - name: compile
  commands:
    - func: "fetch source"
    - func: "run a task that fails"

- name: timeout_test
  depends_on: 
  - name: compile
  commands:
    - func: "fetch source"
    - command: shell.exec
      timeout_secs: 20
      params:
        working_dir: src
        script: |
           echo "this is going to timeout"
           ./run.py timeout
modules:
- name: render-module
  repo: git@github.com:evergreen-ci/render.git
  prefix: modules
  branch: main

buildvariants:
- name: osx-108
  display_name: OSX
  modules: ~
  run_on:
  - localtestdistro
  expansions:
    test_flags: "blah blah"
  tasks:
  - name: compile
  - name: passing_test
  - name: failing_test
  - name: timeout_test
- name: ubuntu
  display_name: Ubuntu
  modules: ["render-module"]
  run_on:
  - ubuntu1404-test
  expansions:
    test_flags: "blah blah"
  tasks:
  - name: compile
  - name: passing_test
  - name: failing_test
  - name: timeout_test`
	require.NoError(t, db.ClearCollections(serviceModel.ParserProjectCollection, serviceModel.ProjectRefCollection, patch.Collection, evergreen.ConfigCollection, task.Collection, serviceModel.VersionCollection, build.Collection))
	require.NoError(t, db.CreateCollections(serviceModel.ParserProjectCollection, build.Collection, task.Collection, serviceModel.VersionCollection, serviceModel.ParserProjectCollection, manifest.Collection))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestSchedulePatchRoute")
	require.NoError(t, settings.Set(ctx))
	projectRef := &serviceModel.ProjectRef{
		Id:         "sample",
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
		RemotePath: "evergreen.yml",
		Enabled:    true,
		BatchTime:  180,
	}
	require.NoError(t, projectRef.Insert())
	unfinalized := patch.Patch{
		Id:                   mgobson.NewObjectId(),
		Project:              projectRef.Id,
		Githash:              "3c7bfeb82d492dc453e7431be664539c35b5db4b",
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
		PatchedProjectConfig: config,
	}
	require.NoError(t, unfinalized.Insert())

	pp := &serviceModel.ParserProject{}
	require.NoError(t, util.UnmarshalYAMLWithFallback([]byte(config), pp))
	pp.Id = unfinalized.Id.Hex()
	require.NoError(t, pp.Insert())

	handler := makeSchedulePatchHandler(env).(*schedulePatchHandler)

	// nonexistent patch ID should error
	req, err := http.NewRequest(http.MethodPost, "", nil)
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": mgobson.NewObjectId().Hex()})
	assert.NoError(t, err)
	assert.Error(t, handler.Parse(ctx, req))

	// valid request, scheduling patch for the first time
	handler = makeSchedulePatchHandler(env).(*schedulePatchHandler)
	description := "some text"
	body := patchTasks{
		Description: description,
		Variants:    []variant{{Id: "ubuntu", Tasks: []string{"compile", "passing_test"}}},
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(t, err)
	req, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": unfinalized.Id.Hex()})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, req))
	resp := handler.Run(ctx)
	respVersion := resp.Data().(model.APIVersion)
	assert.Equal(t, unfinalized.Id.Hex(), *respVersion.Id)
	assert.Equal(t, description, *respVersion.Message)
	tasks, err := task.Find(task.ByVersion(*respVersion.Id))
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	foundCompile := false
	foundPassing := false
	for _, t := range tasks {
		if t.DisplayName == "compile" {
			foundCompile = true
		}
		if t.DisplayName == "passing_test" {
			foundPassing = true
		}
	}
	assert.True(t, foundCompile)
	assert.True(t, foundPassing)

	// valid request, reconfiguring a finalized patch
	handler = makeSchedulePatchHandler(env).(*schedulePatchHandler)
	body = patchTasks{
		Variants: []variant{{Id: "ubuntu", Tasks: []string{"failing_test"}}},
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(t, err)
	req, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": unfinalized.Id.Hex()})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, req))
	resp = handler.Run(ctx)
	respVersion = resp.Data().(model.APIVersion)
	assert.Equal(t, unfinalized.Id.Hex(), *respVersion.Id)
	assert.Equal(t, description, *respVersion.Message)
	tasks, err = task.Find(task.ByVersion(*respVersion.Id))
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
	foundCompile = false
	foundPassing = false
	foundFailing := false
	for _, t := range tasks {
		if t.DisplayName == "compile" {
			foundCompile = true
		}
		if t.DisplayName == "passing_test" {
			foundPassing = true
		}
		if t.DisplayName == "failing_test" {
			foundFailing = true
		}
	}
	assert.True(t, foundFailing)
	assert.True(t, foundPassing)
	assert.True(t, foundCompile)

	// * should select all tasks
	patch2 := patch.Patch{
		Id:                   mgobson.NewObjectId(),
		Project:              projectRef.Id,
		Githash:              "3c7bfeb82d492dc453e7431be664539c35b5db4b",
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
		PatchedProjectConfig: config,
	}
	assert.NoError(t, patch2.Insert())

	err = util.UnmarshalYAMLWithFallback([]byte(config), &pp)
	require.NoError(t, err)
	pp.Id = patch2.Id.Hex()
	require.NoError(t, pp.Insert())

	handler = makeSchedulePatchHandler(env).(*schedulePatchHandler)
	body = patchTasks{
		Variants: []variant{{Id: "ubuntu", Tasks: []string{"*"}}},
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(t, err)
	req, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": patch2.Id.Hex()})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, req))
	resp = handler.Run(ctx)
	respVersion = resp.Data().(model.APIVersion)
	assert.Equal(t, patch2.Id.Hex(), *respVersion.Id)
	assert.Equal(t, "", *respVersion.Message)
	tasks, err = task.Find(task.ByVersion(*respVersion.Id))
	assert.NoError(t, err)
	assert.Len(t, tasks, 4)
}

func TestSchedulePatchActivatesInactiveTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	generatedProject := []string{`
{
  "buildvariants": [
    {
      "name": "testBV1",
      "tasks": [
        {
          "name": "shouldDependOnVersionGen",
          "activate": false
        }
      ],
      "activate": false
    },
    {
      "name": "testBV2",
      "tasks": [
        {
          "name": "shouldDependOnDependencyTask",
          "activate": false
        }
      ],
      "activate": false
    },
    {
      "name": "testBV3",
      "tasks": [
        {
          "name": "shouldDependOnDependencyTask",
          "activate": false
        }
      ],
      "activate": false
    }
  ],
  "tasks": [
    {
      "name": "shouldDependOnVersionGen",
      "commands": [
        {
          "command": "shell.exec",
          "params": {
            "working_dir": "src",
            "script": "echo noop"
          }
        }
      ]
    },
    {
      "name": "shouldDependOnDependencyTask",
      "commands": [
        {
          "command": "shell.exec",
          "params": {
            "working_dir": "src",
            "script": "echo noop"
          }
        }
      ],
      "depends_on": [
        {
          "name": "dependencyTask"
        }
      ]
    }
  ]
}
`}
	const config = `
buildvariants:
  - display_name: Generate Tasks for Version
    name: generate-tasks-for-version
    run_on:
      - ubuntu1604-test
    tasks:
      - name: version_gen

  - display_name: TestBV1
    name: testBV1
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
        depends_on:
          - name: version_gen
            variant: generate-tasks-for-version
            omit_generated_tasks: true

  - name: testBV2
    display_name: TestBV2
    run_on:
      - ubuntu1604-test
    tasks:
      - name: dependencyTask
      - name: placeholder

  - name: testBV3
    display_name: TestBV3
    run_on:
      - ubuntu1604-test
    tasks:
      - name: placeholder
    depends_on:
      - name: version_gen
        variant: generate-tasks-for-version
      - name: dependencyTask
        variant: testBV4

  - name: testBV4
    display_name: TestBV4
    run_on:
      - ubuntu1604-test
    tasks:
      - name: dependencyTask
      - name: dependencyTaskShouldActivate
      - name: shouldNotActivate
      - name: depOfShouldNotActivate
      - name: shouldActivate

tasks:
  - name: placeholder
    depends_on:
        - name: version_gen
          variant: generate-tasks-for-version
    commands:
        - command: shell.exec
          params:
            working_dir: src
            script: |
              echo "noop2"

  - name: dependencyTask
    depends_on:
      - name: shouldNotActivate
    commands:
        - command: shell.exec
          params:
               script: |
                echo "noop2"

  - name: dependencyTaskShouldActivate
    depends_on:
      - name: shouldActivate
    commands:
        - command: shell.exec
          params:
               script: |
                echo "noop2"

  - name: shouldNotActivate
    depends_on:
      - name: depOfShouldNotActivate
    commands:
        - command: shell.exec
          params:
            script: |
                echo "noop2"

  - name: shouldActivate
    commands:
        - command: shell.exec
          params:
            script: |
                echo "noop2"

  - name: depOfShouldNotActivate
    commands:
        - command: shell.exec
          params:
            script: |
                echo "noop2"

  - name: version_gen
    commands:
        - command: generate.tasks
          params:
            files:
              - src/evergreen.json
`
	require.NoError(t, db.ClearCollections(serviceModel.ParserProjectCollection, serviceModel.ProjectRefCollection, patch.Collection, evergreen.ConfigCollection, task.Collection, serviceModel.VersionCollection, build.Collection))
	require.NoError(t, db.CreateCollections(serviceModel.ParserProjectCollection, build.Collection, task.Collection, serviceModel.VersionCollection, serviceModel.ParserProjectCollection, manifest.Collection))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestSchedulePatchRoute")
	require.NoError(t, settings.Set(ctx))
	projectRef := &serviceModel.ProjectRef{
		Id:         "sample",
		Owner:      "evergreen-ci",
		Repo:       "sample",
		Branch:     "main",
		RemotePath: "evergreen.yml",
		Enabled:    true,
		BatchTime:  180,
	}
	require.NoError(t, projectRef.Insert())
	unfinalized := patch.Patch{
		Id:                   mgobson.NewObjectId(),
		Project:              projectRef.Id,
		Githash:              "3c7bfeb82d492dc453e7431be664539c35b5db4b",
		ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
		PatchedProjectConfig: config,
	}
	require.NoError(t, unfinalized.Insert())

	pp := &serviceModel.ParserProject{}
	require.NoError(t, util.UnmarshalYAMLWithFallback([]byte(config), pp))
	pp.Id = unfinalized.Id.Hex()
	require.NoError(t, pp.Insert())

	// schedule patch with task generator for the first run
	handler := makeSchedulePatchHandler(env).(*schedulePatchHandler)
	description := "some text"
	body := patchTasks{
		Description: description,
		Variants:    []variant{{Id: "generate-tasks-for-version", Tasks: []string{"version_gen"}}},
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": unfinalized.Id.Hex()})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, req))
	resp := handler.Run(ctx)
	respVersion := resp.Data().(model.APIVersion)
	assert.Equal(t, unfinalized.Id.Hex(), *respVersion.Id)
	assert.Equal(t, description, *respVersion.Message)
	tasks, err := task.Find(task.ByVersion(*respVersion.Id))
	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	// manually set the task as running and its generated JSON for simplicity
	err = task.UpdateOne(task.ById(tasks[0].Id), bson.M{
		"$set": bson.M{
			task.StatusKey:                evergreen.TaskStarted,
			task.GeneratedJSONAsStringKey: generatedProject,
		}})
	assert.NoError(t, err)
	j := units.NewGenerateTasksJob(tasks[0].Version, tasks[0].Id, "1")
	j.Run(context.Background())
	assert.NoError(t, j.Error())

	// now re-configure with tasks that have already been generated but are inactive
	// this task has two dependencies which should also be activated
	handler = makeSchedulePatchHandler(env).(*schedulePatchHandler)
	body = patchTasks{
		Variants: []variant{{Id: "testBV4", Tasks: []string{"dependencyTask"}}},
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(t, err)
	req, err = http.NewRequest(http.MethodPost, "", bytes.NewBuffer(jsonBody))
	req = gimlet.SetURLVars(req, map[string]string{"patch_id": unfinalized.Id.Hex()})
	assert.NoError(t, err)
	assert.NoError(t, handler.Parse(ctx, req))
	resp = handler.Run(ctx)
	respVersion = resp.Data().(model.APIVersion)
	assert.Equal(t, unfinalized.Id.Hex(), *respVersion.Id)
	assert.Equal(t, description, *respVersion.Message)
	tasks, err = task.Find(task.ByVersion(*respVersion.Id))
	assert.NoError(t, err)
	// affirm that pre-existing inactive tasks are all activated now
	for _, foundTask := range tasks {
		if foundTask.BuildVariant == "testBV4" {
			assert.True(t, foundTask.Activated)
		}
	}
}
