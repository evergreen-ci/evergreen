package data

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	dbModel "github.com/evergreen-ci/evergreen/model"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by project route

type PatchConnectorFetchByProjectSuite struct {
	ctx      Connector
	time     time.Time
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorFetchByProjectSuite(t *testing.T) {
	s := new(PatchConnectorFetchByProjectSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

		patches := []*patch.Patch{
			{Project: "project1", CreateTime: s.time},
			{Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
			{Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
			{Project: "project3", CreateTime: s.time.Add(time.Second * 12)},
		}

		for _, p := range patches {
			if err := p.Insert(); err != nil {
				return err
			}
		}
		pRef1 := dbModel.ProjectRef{Id: "project1", Identifier: "project_one"}
		pRef2 := dbModel.ProjectRef{Id: "project2", Identifier: "project_two"}
		pRef3 := dbModel.ProjectRef{Id: "project3", Identifier: "project_three"}
		pRef4 := dbModel.ProjectRef{Id: "project4", Identifier: "project_four"}
		assert.NoError(t, pRef1.Insert())
		assert.NoError(t, pRef2.Insert())
		assert.NoError(t, pRef3.Insert())
		assert.NoError(t, pRef4.Insert())
		return nil
	}

	s.teardown = func() error {
		return db.ClearCollections(patch.Collection, dbModel.ProjectRefCollection)
	}

	suite.Run(t, s)
}

func TestMockPatchConnectorFetchByProjectSuite(t *testing.T) {
	s := new(PatchConnectorFetchByProjectSuite)
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
		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []model.APIPatch{
				{ProjectId: &proj1, CreateTime: &s.time},
				{ProjectId: &proj2, CreateTime: &nowPlus2},
				{ProjectId: &proj1, CreateTime: &nowPlus4},
				{ProjectId: &proj1, CreateTime: &nowPlus6},
				{ProjectId: &proj2, CreateTime: &nowPlus8},
				{ProjectId: &proj1, CreateTime: &nowPlus10},
				{ProjectId: &proj3, ProjectIdentifier: &proj3Identifier, CreateTime: &nowPlus12},
			},
			CachedProjectRefs: []model.APIProjectRef{
				{Id: &proj1, Identifier: &proj1Identifier},
				{Id: &proj2, Identifier: &proj2Identifier},
				{Id: &proj3, Identifier: &proj3Identifier},
				{Id: &proj4, Identifier: &proj4Identifier},
			},
		},
		}

		return nil
	}

	s.teardown = func() error { return nil }

	suite.Run(t, s)
}

func (s *PatchConnectorFetchByProjectSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *PatchConnectorFetchByProjectSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooManyAsc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	if s.Len(patches, 2) {
		s.Equal("project2", *patches[0].ProjectId)
		s.Equal("project2", *patches[1].ProjectId)
		s.True(patches[1].CreateTime.Before(*patches[0].CreateTime))
	}
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooManyDesc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	if s.Len(patches, 2) {
		s.Equal("project2", *patches[0].ProjectId)
		s.Equal("project2", *patches[1].ProjectId)
		s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	}
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("project2", *patches[0].ProjectId)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooFew() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestProjectNonexistentFail() {
	_, err := s.ctx.FindPatchesByProject("zzz", s.time, 1)
	s.Error(err)
}

func (s *PatchConnectorFetchByProjectSuite) TestEmptyPatchesOkay() {
	patches, err := s.ctx.FindPatchesByProject("project4", s.time, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyWithinBound() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second*6), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyOutOfBound() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(-time.Hour), 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFindPatchesByIdentifier() {
	patches, err := s.ctx.FindPatchesByProject("project_three", s.time.Add(time.Second*14), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal("project3", *patches[0].ProjectId)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchConnectorFetchByIdSuite struct {
	ctx      Connector
	obj_ids  []string
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorFetchByIdSuite(t *testing.T) {
	s := new(PatchConnectorFetchByIdSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		patches := []patch.Patch{
			{Id: mgobson.ObjectIdHex(s.obj_ids[0])},
			{Id: mgobson.ObjectIdHex(s.obj_ids[1])},
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

	suite.Run(t, s)
}

func TestMockPatchConnectorFetchByIdSuite(t *testing.T) {
	s := new(PatchConnectorFetchByIdSuite)
	s.setup = func() error {

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []model.APIPatch{
				{Id: &s.obj_ids[0]},
				{Id: &s.obj_ids[1]},
			},
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	suite.Run(t, s)
}

func (s *PatchConnectorFetchByIdSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *PatchConnectorFetchByIdSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorFetchByIdSuite) TestFetchById() {
	p, err := s.ctx.FindPatchById(s.obj_ids[0])
	s.Require().NoError(err)
	s.Require().NotNil(p)
	s.Equal(s.obj_ids[0], *p.Id)
}

func (s *PatchConnectorFetchByIdSuite) TestFetchByIdFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	p, err := s.ctx.FindPatchById(new_id.Hex())
	s.Error(err)
	s.Nil(p)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort patch by id route

type PatchConnectorAbortByIdSuite struct {
	ctx      Connector
	obj_ids  []string
	mock     bool
	setup    func() error
	teardown func() error
	prBody   []byte

	suite.Suite
}

func TestPatchConnectorAbortByIdSuite(t *testing.T) {
	s := new(PatchConnectorAbortByIdSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		patches := []*patch.Patch{
			{Id: mgobson.ObjectIdHex(s.obj_ids[0]), Version: "version1"},
			{Id: mgobson.ObjectIdHex(s.obj_ids[1])},
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

	s.mock = false
	suite.Run(t, s)
}

func TestMockPatchConnectorAbortByIdSuite(t *testing.T) {
	s := new(PatchConnectorAbortByIdSuite)
	s.setup = func() error {

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []model.APIPatch{
				{Id: &s.obj_ids[0], Version: utility.ToStringPtr("version1")},
				{Id: &s.obj_ids[1]},
			},
			CachedAborted: make(map[string]string),
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	s.mock = true
	suite.Run(t, s)
}

func (s *PatchConnectorAbortByIdSuite) SetupSuite() {
	s.Require().NoError(s.setup())
	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "..", "route", "testdata", "pull_request.json"))
	s.NoError(err)
	s.Len(s.prBody, 24731)
}

func (s *PatchConnectorAbortByIdSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorAbortByIdSuite) TestAbort() {
	err := s.ctx.AbortPatch(s.obj_ids[0], "user1")
	s.NoError(err)
	p, err := s.ctx.FindPatchById(s.obj_ids[0])
	s.Require().NoError(err)
	s.Require().NotNil(p)
	s.Equal(s.obj_ids[0], *p.Id)
	if s.mock {
		s.Equal("user1", s.ctx.(*MockConnector).MockPatchConnector.CachedAborted[s.obj_ids[0]])
	}

	err = s.ctx.AbortPatch(s.obj_ids[1], "user1")
	s.NoError(err)

	p, err = s.ctx.FindPatchById(s.obj_ids[1])

	s.Error(err)
	s.Nil(p)
}

func (s *PatchConnectorAbortByIdSuite) TestAbortFail() {
	new_id := mgobson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	err := s.ctx.AbortPatch(new_id.Hex(), "user")
	s.Error(err)
}

func (s *PatchConnectorAbortByIdSuite) TestAbortByPullRequest() {
	eventInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	event, ok := eventInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.Contains(s.ctx.AbortPatchesFromPullRequest(event).Error(), "pull request data is malformed")

	now := time.Now().Round(time.Millisecond)
	event.PullRequest.ClosedAt = &now
	s.NoError(s.ctx.AbortPatchesFromPullRequest(event))
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

	now := time.Now().Round(time.Millisecond)
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
	ctx      Connector
	obj_ids  []string
	mock     bool
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorChangeStatusSuite(t *testing.T) {
	s := new(PatchConnectorChangeStatusSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		patches := []*patch.Patch{
			{Id: mgobson.ObjectIdHex(s.obj_ids[0]), Version: s.obj_ids[0]},
			{Id: mgobson.ObjectIdHex(s.obj_ids[1]), Version: s.obj_ids[1]},
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

	s.mock = false
	suite.Run(t, s)
}

func TestMockPatchConnectorChangeStatusSuite(t *testing.T) {
	s := new(PatchConnectorChangeStatusSuite)
	s.setup = func() error {

		s.obj_ids = []string{mgobson.NewObjectId().Hex(), mgobson.NewObjectId().Hex()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []model.APIPatch{
				{Id: &s.obj_ids[0], Version: &s.obj_ids[0]},
				{Id: &s.obj_ids[1], Version: &s.obj_ids[1]},
			},
			CachedAborted:  make(map[string]string),
			CachedPriority: make(map[string]int64),
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	s.mock = true
	suite.Run(t, s)
}

func (s *PatchConnectorChangeStatusSuite) SetupSuite() {
	s.Require().NoError(s.setup())
}

func (s *PatchConnectorChangeStatusSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorChangeStatusSuite) TestSetPriority() {
	p, err := s.ctx.FindPatchById(s.obj_ids[0])
	s.NoError(err)
	err = s.ctx.SetPatchPriority(s.obj_ids[0], 7, "")
	s.NoError(err)
	if s.mock {
		s.Equal(int64(7), s.ctx.(*MockConnector).MockPatchConnector.CachedPriority[*p.Id])
	}
}

func (s *PatchConnectorChangeStatusSuite) TestSetActivation() {
	settings := testutil.MockConfig()
	err := s.ctx.SetPatchActivated(context.Background(), s.obj_ids[0], "user1", true, settings)
	s.NoError(err)
	p, err := s.ctx.FindPatchById(s.obj_ids[0])
	s.NoError(err)
	s.Require().NotNil(p)
	s.True(p.Activated)

	err = s.ctx.SetPatchActivated(context.Background(), s.obj_ids[0], "user1", false, settings)
	s.NoError(err)
	p, err = s.ctx.FindPatchById(s.obj_ids[0])
	s.NoError(err)
	s.False(p.Activated)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patches for current user route

type PatchConnectorFetchByUserSuite struct {
	ctx  Connector
	time time.Time

	suite.Suite
}

func TestPatchConnectorFetchByUserSuite(t *testing.T) {
	s := new(PatchConnectorFetchByUserSuite)
	s.ctx = &DBConnector{}
	s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

	assert.NoError(t, db.Clear(patch.Collection))

	patches := []*patch.Patch{
		{Author: "user1", CreateTime: s.time},
		{Author: "user2", CreateTime: s.time.Add(time.Second * 2)},
		{Author: "user1", CreateTime: s.time.Add(time.Second * 4)},
		{Author: "user1", CreateTime: s.time.Add(time.Second * 6)},
		{Author: "user2", CreateTime: s.time.Add(time.Second * 8)},
		{Author: "user1", CreateTime: s.time.Add(time.Second * 10)},
	}

	for _, p := range patches {
		assert.NoError(t, p.Insert())
	}

	suite.Run(t, s)
}

func TestMockPatchConnectorFetchByUserSuite(t *testing.T) {
	s := new(PatchConnectorFetchByUserSuite)

	s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)
	user1 := "user1"
	user2 := "user2"
	nowPlus2 := s.time.Add(time.Second * 2)
	nowPlus4 := s.time.Add(time.Second * 4)
	nowPlus6 := s.time.Add(time.Second * 6)
	nowPlus8 := s.time.Add(time.Second * 8)
	nowPlus10 := s.time.Add(time.Second * 10)
	s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
		CachedPatches: []model.APIPatch{
			{Author: &user1, CreateTime: &s.time},
			{Author: &user2, CreateTime: &nowPlus2},
			{Author: &user1, CreateTime: &nowPlus4},
			{Author: &user1, CreateTime: &nowPlus6},
			{Author: &user2, CreateTime: &nowPlus8},
			{Author: &user1, CreateTime: &nowPlus10},
		},
	},
	}

	suite.Run(t, s)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooMany() {
	patches, err := s.ctx.FindPatchesByUser("user2", s.time.Add(time.Second*10), 3)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("user2", *patches[0].Author)
	s.Equal("user2", *patches[1].Author)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
}

func (s *PatchConnectorFetchByUserSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByUser("user2", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	if s.NotNil(patches) && s.Len(patches, 1) {
		s.Equal("user2", *patches[0].Author)
	}
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooFew() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Second*10), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), *patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchNonexistentFail() {
	patches, err := s.ctx.FindPatchesByUser("zzz", s.time, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyWithinBound() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Second*6), 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), *patches[0].CreateTime)
}
func (s *PatchConnectorFetchByUserSuite) TestFetchKeyOutOfBound() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(-time.Hour), 1)
	s.NoError(err)
	s.Len(patches, 0)
}

type PatchConnectorFindByUserPatchNameStatusesCommitQueue struct {
	ctx      Connector
	time     time.Time
	setup    func() error
	teardown func() error
	obj_ids  []string
	suite.Suite
}

func TestPatchConnectorFindByUserPatchNameStatusesCommitQueue(t *testing.T) {
	s := new(PatchConnectorFindByUserPatchNameStatusesCommitQueue)
	s.setup = func() error {
		s.ctx = &DBConnector{}
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)
		s.obj_ids = []string{
			mgobson.NewObjectId().Hex(),
			mgobson.NewObjectId().Hex(),
			mgobson.NewObjectId().Hex(),
			mgobson.NewObjectId().Hex(),
			mgobson.NewObjectId().Hex(),
			mgobson.NewObjectId().Hex(),
		}
		patches := []*patch.Patch{
			{Id: mgobson.ObjectIdHex(s.obj_ids[0]), Author: "user1", CreateTime: s.time, Description: "user 1 patch 1", Alias: evergreen.CommitQueueAlias, Status: evergreen.PatchCreated},
			{Id: mgobson.ObjectIdHex(s.obj_ids[1]), Author: "user2", CreateTime: s.time.Add(time.Second * 2), Description: "user 2 patch 1", Alias: evergreen.CommitQueueAlias, Status: evergreen.PatchStarted},
			{Id: mgobson.ObjectIdHex(s.obj_ids[2]), Author: "user1", CreateTime: s.time.Add(time.Second * 4), Description: "user 1 patch 2 llama", Alias: evergreen.GithubPRAlias, Status: evergreen.PatchSucceeded},
			{Id: mgobson.ObjectIdHex(s.obj_ids[3]), Author: "user1", CreateTime: s.time.Add(time.Second * 6), Description: "user 1 patch 3", Alias: evergreen.CommitQueueAlias, Status: evergreen.PatchFailed},
			{Id: mgobson.ObjectIdHex(s.obj_ids[4]), Author: "user2", CreateTime: s.time.Add(time.Second * 8), Description: "user 2 patch 2", Alias: evergreen.CommitQueueAlias, Status: evergreen.PatchStarted},
			{Id: mgobson.ObjectIdHex(s.obj_ids[5]), Author: "user1", CreateTime: s.time.Add(time.Second * 10), Description: "user 1 patch 4 llama", Alias: evergreen.CommitQueueAlias, Status: evergreen.PatchFailed},
		}
		assert.NoError(t, db.Clear(patch.Collection))
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
	suite.Run(t, s)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) SetupSuite() {
	s.Require().NoError(s.setup())
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchAllPatchesForUser() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user2", "", []string{}, true, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal(s.obj_ids[4], *patches[0].Id)
	s.Equal(s.obj_ids[1], *patches[1].Id)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	s.Equal(*count, 2)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchPatchesForUserAndFilterByCommitQueue() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user2", "", []string{}, false, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 0)
	s.Equal(*count, 0)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchPatchesForUserAndFilterByStatuses() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "", []string{evergreen.PatchCreated}, true, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(evergreen.PatchCreated, *patches[0].Status)
	s.Equal(*count, 1)

	patches, count, err = s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "", []string{evergreen.PatchFailed, evergreen.PatchCreated}, true, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 3)
	s.Equal(s.obj_ids[5], *patches[0].Id)
	s.Equal(s.obj_ids[3], *patches[1].Id)
	s.Equal(s.obj_ids[0], *patches[2].Id)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	s.True(patches[1].CreateTime.After(*patches[2].CreateTime))
	s.Equal(*count, 3)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchPatchesForUserAndFilterByPatchName() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "llama", []string{}, true, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal(s.obj_ids[5], *patches[0].Id)
	s.Equal(s.obj_ids[2], *patches[1].Id)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	s.Equal(*count, 2)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchAllPatchesForUserAndCombineFilters() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "llama", []string{evergreen.PatchSucceeded}, true, 0, 0)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.obj_ids[2], *patches[0].Id)
	s.Equal(*count, 1)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchAllPatchesForUserAndPaginate() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "", []string{}, true, 0, 2)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal(s.obj_ids[5], *patches[0].Id)
	s.Equal(s.obj_ids[3], *patches[1].Id)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	s.Equal(*count, 4)

	patches, count, err = s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "", []string{}, true, 1, 2)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal(s.obj_ids[2], *patches[0].Id)
	s.Equal(s.obj_ids[0], *patches[1].Id)
	s.True(patches[0].CreateTime.After(*patches[1].CreateTime))
	s.Equal(*count, 4)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TestFetchPatchesForUserFilterAndPaginate() {
	patches, count, err := s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "llama", []string{}, true, 0, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.obj_ids[5], *patches[0].Id)
	s.Equal(*count, 2)

	patches, count, err = s.ctx.FindPatchesByUserPatchNameStatusesCommitQueue("user1", "llama", []string{}, true, 1, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.obj_ids[2], *patches[0].Id)
	s.Equal(*count, 2)
}

func (s *PatchConnectorFindByUserPatchNameStatusesCommitQueue) TearDownSuite() {
	s.Require().NoError(s.teardown())
}
