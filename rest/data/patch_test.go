package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type PatchConnectorFetchByProjectSuite struct {
	time     time.Time
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorFetchByProjectSuite(t *testing.T) {
	s := new(PatchConnectorFetchByProjectSuite)
	suite.Run(t, s)
}

func (s *PatchConnectorFetchByProjectSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, dbModel.ProjectRefCollection))
	s.setup = func() error {
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

		proj1 := "project1"
		proj1Identifier := "project_one"
		proj2 := "project2"
		proj2Identifier := "project_two"
		proj3 := "project3"
		proj3Identifier := "project_three"
		proj4 := "project4"
		proj4Identifier := "project_four"
		nowPlus2 := s.time.Add(time.Second * 2)
		nowPlus4 := s.time.Add(time.Second * 4)
		nowPlus6 := s.time.Add(time.Second * 6)
		nowPlus8 := s.time.Add(time.Second * 8)
		nowPlus10 := s.time.Add(time.Second * 10)
		nowPlus12 := s.time.Add(time.Second * 12)
		projects := []dbModel.ProjectRef{
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
			{
				Id:         proj4,
				Identifier: proj4Identifier,
			},
		}
		patches := []patch.Patch{
			{Project: proj1, CreateTime: s.time},
			{Project: proj2, CreateTime: nowPlus2},
			{Project: proj1, CreateTime: nowPlus4},
			{Project: proj1, CreateTime: nowPlus6},
			{Project: proj2, CreateTime: nowPlus8},
			{Project: proj1, CreateTime: nowPlus10},
			{Project: proj3, CreateTime: nowPlus12},
		}
		for _, p := range patches {
			s.NoError(p.Insert())
		}
		for _, proj := range projects {
			s.NoError(proj.Insert())
		}

		return nil
	}

	s.teardown = func() error { return db.Clear(patch.Collection) }
	s.Require().NoError(s.setup())
}

func (s *PatchConnectorFetchByProjectSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooManyAsc() {
	patches, err := FindPatchesByProject("project2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	if s.Len(patches, 2) {
		s.Equal("project2", *patches[0].ProjectId)
		s.Equal("project2", *patches[1].ProjectId)
		s.True(patches[1].CreateTime.Before(*patches[0].CreateTime))
	}
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooManyDesc() {
	patches, err := FindPatchesByProject("project2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	if s.Len(patches, 2) {
		s.Equal("project2", *patches[0].ProjectId)
		s.Equal("project2", *patches[1].ProjectId)
		s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	}
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchExactNumber() {
	patches, err := FindPatchesByProject("project2", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("project2", *patches[0].ProjectId)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooFew() {
	patches, err := FindPatchesByProject("project1", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestProjectNonexistentFail() {
	_, err := FindPatchesByProject("zzz", s.time, 1)
	s.Error(err)
}

func (s *PatchConnectorFetchByProjectSuite) TestEmptyPatchesOkay() {
	patches, err := FindPatchesByProject("project4", s.time, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyWithinBound() {
	patches, err := FindPatchesByProject("project1", s.time.Add(time.Second*6), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyOutOfBound() {
	patches, err := FindPatchesByProject("project1", s.time.Add(-time.Hour), 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFindPatchesByIdentifier() {
	patches, err := FindPatchesByProject("project_three", s.time.Add(time.Second*14), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal("project3", *patches[0].ProjectId)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchConnectorFetchByIdSuite struct {
	obj_ids  []string
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorFetchByIdSuite(t *testing.T) {
	s := new(PatchConnectorFetchByIdSuite)
	suite.Run(t, s)
}

func (s *PatchConnectorFetchByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, dbModel.ProjectRefCollection, dbModel.VersionCollection))
	s.setup = func() error {
		version := dbModel.Version{
			Id: "version1",
		}
		s.NoError(version.Insert())

		s.obj_ids = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}
		patches := []patch.Patch{
			{Id: patch.NewId(s.obj_ids[0])},
			{Id: patch.NewId(s.obj_ids[1])},
		}

		for _, p := range patches {
			if err := p.Insert(); err != nil {
				return err
			}
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(patch.Collection)
	}

	s.Require().NoError(s.setup())
}

func (s *PatchConnectorFetchByIdSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorFetchByIdSuite) TestFetchById() {
	p, err := FindPatchById(s.obj_ids[0])
	s.Require().NoError(err)
	s.Require().NotNil(p)
	s.Equal(s.obj_ids[0], *p.Id)
}

func (s *PatchConnectorFetchByIdSuite) TestFetchByIdFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	p, err := FindPatchById(new_id.Hex())
	s.Error(err)
	s.Nil(p)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort patch by id route

type PatchConnectorAbortByIdSuite struct {
	obj_ids  []string
	setup    func() error
	teardown func() error
	prBody   []byte

	suite.Suite
}

func TestPatchConnectorAbortByIdSuite(t *testing.T) {
	s := new(PatchConnectorAbortByIdSuite)
	suite.Run(t, s)
}

func (s *PatchConnectorAbortByIdSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, dbModel.ProjectRefCollection, dbModel.VersionCollection))
	s.setup = func() error {

		version := dbModel.Version{
			Id: "version1",
		}
		s.NoError(version.Insert())
		s.obj_ids = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}
		patches := []patch.Patch{
			{Id: patch.NewId(s.obj_ids[0]), Version: version.Id},
			{Id: patch.NewId(s.obj_ids[1])},
		}
		for _, p := range patches {
			s.NoError(p.Insert())
		}

		return nil
	}

	s.teardown = func() error { return nil }

	s.Require().NoError(s.setup())
	var err error
	s.prBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "..", "route", "testdata", "pull_request.json"))
	s.NoError(err)
	s.Len(s.prBody, 24692)
}

func (s *PatchConnectorAbortByIdSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorAbortByIdSuite) TestAbort() {
	err := AbortPatch(context.Background(), s.obj_ids[0], "user1")
	s.NoError(err)
	p, err := FindPatchById(s.obj_ids[0])
	s.Require().NoError(err)
	s.Require().NotNil(p)
	s.Equal(s.obj_ids[0], *p.Id)
	abortedPatch, err := FindPatchById(s.obj_ids[0])
	s.NoError(err)
	s.Equal(evergreen.PatchVersionRequester, *abortedPatch.Requester)

	err = AbortPatch(context.Background(), s.obj_ids[1], "user1")
	s.NoError(err)

	p, err = FindPatchById(s.obj_ids[1])

	s.Error(err)
	s.Nil(p)
}

func (s *PatchConnectorAbortByIdSuite) TestAbortFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	err := AbortPatch(context.Background(), new_id.Hex(), "user")
	s.Error(err)
}

func (s *PatchConnectorAbortByIdSuite) TestAbortByPullRequest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	event, ok := eventInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.Contains(AbortPatchesFromPullRequest(ctx, event).Error(), "pull request data is malformed")

	now := github.Timestamp{
		Time: time.Now().Round(time.Millisecond),
	}
	event.PullRequest.ClosedAt = &now
	s.NoError(AbortPatchesFromPullRequest(ctx, event))
}

func (s *PatchConnectorAbortByIdSuite) TestVerifyPullRequestEventForAbort() {
	eventInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	event, ok := eventInterface.(*github.PullRequestEvent)
	s.True(ok)
	owner, repo, err := verifyPullRequestEventForAbort(event)
	s.Empty(owner)
	s.Empty(repo)
	s.Contains(err.Error(), "pull request data is malformed")

	now := github.Timestamp{
		Time: time.Now().Round(time.Millisecond),
	}
	event.PullRequest.ClosedAt = &now
	event.Repo.FullName = github.String("somethingmalformed")
	owner, repo, err = verifyPullRequestEventForAbort(event)
	s.Error(err)
	s.Empty(owner)
	s.Empty(repo)

	event.Repo.FullName = github.String("baxterthehacker/public-repo")
	owner, repo, err = verifyPullRequestEventForAbort(event)
	s.NoError(err)
	s.Equal("baxterthehacker", owner)
	s.Equal("public-repo", repo)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change patch status route

type PatchConnectorChangeStatusSuite struct {
	obj_ids  []string
	DB       bool
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorChangeStatusSuite(t *testing.T) {
	s := new(PatchConnectorChangeStatusSuite)
	suite.Run(t, s)
}

func (s *PatchConnectorChangeStatusSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection, dbModel.ProjectRefCollection, task.Collection, dbModel.VersionCollection))
	s.setup = func() error {
		s.obj_ids = []string{"aabbccddeeff001122334455", "aabbccddeeff001122334456"}

		patches := []*patch.Patch{
			{Id: patch.NewId(s.obj_ids[0]), Version: s.obj_ids[0]},
			{Id: patch.NewId(s.obj_ids[1]), Version: s.obj_ids[1]},
		}
		task := task.Task{
			Id:      "t1",
			Version: s.obj_ids[0],
		}
		s.NoError(task.Insert())
		for _, p := range patches {
			s.NoError(p.Insert())
		}
		for _, id := range s.obj_ids {
			version := dbModel.Version{
				Id: id,
			}
			s.NoError(version.Insert())
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(patch.Collection)
	}

	s.Require().NoError(s.setup())
}

func (s *PatchConnectorChangeStatusSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorChangeStatusSuite) TestSetActivation() {
	settings := testutil.MockConfig()
	err := SetPatchActivated(context.Background(), s.obj_ids[0], "user1", true, settings)
	s.NoError(err)
	p, err := FindPatchById(s.obj_ids[0])
	s.NoError(err)
	s.Require().NotNil(p)
	s.True(p.Activated)

	err = SetPatchActivated(context.Background(), s.obj_ids[0], "user1", false, settings)
	s.NoError(err)
	p, err = FindPatchById(s.obj_ids[0])
	s.NoError(err)
	s.False(p.Activated)

	err = SetPatchActivated(context.Background(), "aabbcccceeee001122334456", "user1", true, settings)
	s.Error(err)
	s.Contains(err.Error(), "could not find patch")
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patches for current user route

type PatchConnectorFetchByUserSuite struct {
	time time.Time

	suite.Suite
}

func TestPatchConnectorFetchByUserSuite(t *testing.T) {
	s := new(PatchConnectorFetchByUserSuite)
	suite.Run(t, s)
}

func (s *PatchConnectorFetchByUserSuite) SetupSuite() {
	s.NoError(db.ClearCollections(patch.Collection))
	s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)
	user1 := "user1"
	user2 := "user2"
	nowPlus2 := s.time.Add(time.Second * 2)
	nowPlus4 := s.time.Add(time.Second * 4)
	nowPlus6 := s.time.Add(time.Second * 6)
	nowPlus8 := s.time.Add(time.Second * 8)
	nowPlus10 := s.time.Add(time.Second * 10)
	patches := []patch.Patch{
		{Author: user1, CreateTime: s.time},
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

func (s *PatchConnectorFetchByUserSuite) TestFetchTooMany() {
	patches, err := FindPatchesByUser("user2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("user2", *patches[0].Author)
	s.Equal("user2", *patches[1].Author)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
}

func (s *PatchConnectorFetchByUserSuite) TestFetchExactNumber() {
	patches, err := FindPatchesByUser("user2", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	if s.NotNil(patches) && s.Len(patches, 1) {
		s.Equal("user2", *patches[0].Author)
	}
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooFew() {
	patches, err := FindPatchesByUser("user1", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchNonexistentFail() {
	patches, err := FindPatchesByUser("zzz", s.time, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyWithinBound() {
	patches, err := FindPatchesByUser("user1", s.time.Add(time.Second*6), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyOutOfBound() {
	patches, err := FindPatchesByUser("user1", s.time.Add(-time.Hour), 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func TestGetRawPatches(t *testing.T) {
	assert.NoError(t, db.Clear(patch.Collection))
	p := patch.Patch{
		Id:      mgobson.NewObjectId(),
		Githash: "hashbrown",
		Patches: []patch.ModulePatch{
			{ModuleName: "different", Githash: "home fries"},
		},
	}
	assert.NoError(t, p.Insert())
	raw, err := GetRawPatches(p.Id.Hex())
	assert.NoError(t, err)
	// Verify that we populate the raw patch patch githash regardless of whether we have changes.
	assert.Equal(t, p.Githash, raw.Patch.Githash)
	assert.Equal(t, "", raw.Patch.Name)
	require.Len(t, raw.RawModules, 1)
	assert.Equal(t, raw.RawModules[0].Name, p.Patches[0].ModuleName)
	assert.Equal(t, raw.RawModules[0].Githash, p.Patches[0].Githash)
}
