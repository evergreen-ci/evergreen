package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
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

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorFetchByProjectSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		patches := []*patch.Patch{
			{Project: "project1", CreateTime: s.time},
			{Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
			{Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
			{Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
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

func TestMockPatchConnectorFetchByProjectSuite(t *testing.T) {
	s := new(PatchConnectorFetchByProjectSuite)
	s.setup = func() error {
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Project: "project1", CreateTime: s.time},
				{Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
				{Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
				{Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
				{Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
				{Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
			},
		}}

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
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 3, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.Before(patches[1].CreateTime))
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooManyDesc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Second*10), 3, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.After(patches[1].CreateTime))
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 1, true)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("project2", patches[0].Project)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooFewAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time, 1, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*4), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchTooFewDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second*10), 1, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchNonexistentFail() {
	patches, err := s.ctx.FindPatchesByProject("zzz", s.time, 1, true)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyWithinBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second*4), 1, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyWithinBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second*6), 1, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyOutOfBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Hour), 1, true)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByProjectSuite) TestFetchKeyOutOfBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(-time.Hour), 1, false)
	s.NoError(err)
	s.Len(patches, 0)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch patch by id route

type PatchConnectorFetchByIdSuite struct {
	ctx      Connector
	obj_ids  []bson.ObjectId
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorFetchByIdSuite(t *testing.T) {
	s := new(PatchConnectorFetchByIdSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorFetchByIdSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		patches := []*patch.Patch{
			{Id: s.obj_ids[0]},
			{Id: s.obj_ids[1]},
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

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Id: s.obj_ids[0]},
				{Id: s.obj_ids[1]},
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
	p, err := s.ctx.FindPatchById(s.obj_ids[0].Hex())
	s.NoError(err)
	s.NotNil(p)
	s.Equal(s.obj_ids[0], p.Id)
}

func (s *PatchConnectorFetchByIdSuite) TestFetchByIdFail() {
	new_id := bson.NewObjectId()
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
	obj_ids  []bson.ObjectId
	mock     bool
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorAbortByIdSuite(t *testing.T) {
	s := new(PatchConnectorAbortByIdSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorAbortByIdSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		patches := []*patch.Patch{
			{Id: s.obj_ids[0], Version: "version1"},
			{Id: s.obj_ids[1]},
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

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Id: s.obj_ids[0], Version: "version1"},
				{Id: s.obj_ids[1]},
			},
			CachedAborted: make(map[string]string),
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	s.mock = true
	suite.Run(t, s)
}

func (s *PatchConnectorAbortByIdSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *PatchConnectorAbortByIdSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorAbortByIdSuite) TestAbort() {
	err := s.ctx.AbortPatch(s.obj_ids[0].Hex(), "user1")
	s.NoError(err)
	p, err := s.ctx.FindPatchById(s.obj_ids[0].Hex())
	s.NoError(err)
	s.NotNil(p)
	s.Equal(s.obj_ids[0], p.Id)
	if s.mock {
		s.Equal("user1", s.ctx.(*MockConnector).MockPatchConnector.CachedAborted[s.obj_ids[0].Hex()])
	}

	err = s.ctx.AbortPatch(s.obj_ids[1].Hex(), "user1")
	s.NoError(err)

	p, err = s.ctx.FindPatchById(s.obj_ids[1].Hex())

	s.Error(err)
	s.Nil(p)
}

func (s *PatchConnectorAbortByIdSuite) TestAbortFail() {
	new_id := bson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	err := s.ctx.AbortPatch(new_id.Hex(), "user")
	s.Error(err)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for change patch status route

type PatchConnectorChangeStatusSuite struct {
	ctx      Connector
	obj_ids  []bson.ObjectId
	mock     bool
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorChangeStatusSuite(t *testing.T) {
	s := new(PatchConnectorChangeStatusSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorAbortByIdSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		patches := []*patch.Patch{
			{Id: s.obj_ids[0]},
			{Id: s.obj_ids[1]},
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

		s.obj_ids = []bson.ObjectId{bson.NewObjectId(), bson.NewObjectId()}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Id: s.obj_ids[0]},
				{Id: s.obj_ids[1]},
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
	p, err := s.ctx.FindPatchById(s.obj_ids[0].Hex())
	s.NoError(err)
	err = s.ctx.SetPatchPriority(s.obj_ids[0].Hex(), 7)
	s.NoError(err)
	if s.mock {
		s.Equal(int64(7), s.ctx.(*MockConnector).MockPatchConnector.CachedPriority[p.Id.Hex()])
	}
}

func (s *PatchConnectorChangeStatusSuite) TestSetActivation() {
	err := s.ctx.SetPatchActivated(s.obj_ids[0].Hex(), "user1", true)
	p, err := s.ctx.FindPatchById(s.obj_ids[0].Hex())
	s.NoError(err)
	s.True(p.Activated)

	err = s.ctx.SetPatchActivated(s.obj_ids[0].Hex(), "user1", false)
	p, err = s.ctx.FindPatchById(s.obj_ids[0].Hex())
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

	testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorFetchByUserSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

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

	s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Author: "user1", CreateTime: s.time},
			{Author: "user2", CreateTime: s.time.Add(time.Second * 2)},
			{Author: "user1", CreateTime: s.time.Add(time.Second * 4)},
			{Author: "user1", CreateTime: s.time.Add(time.Second * 6)},
			{Author: "user2", CreateTime: s.time.Add(time.Second * 8)},
			{Author: "user1", CreateTime: s.time.Add(time.Second * 10)},
		},
	}}

	suite.Run(t, s)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooManyAsc() {
	patches, err := s.ctx.FindPatchesByUser("user2", s.time, 3, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("user2", patches[0].Author)
	s.Equal("user2", patches[1].Author)
	s.True(patches[0].CreateTime.Before(patches[1].CreateTime))
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooManyDesc() {
	patches, err := s.ctx.FindPatchesByUser("user2", s.time.Add(time.Second*10), 3, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("user2", patches[0].Author)
	s.Equal("user2", patches[1].Author)
	s.True(patches[0].CreateTime.After(patches[1].CreateTime))
}

func (s *PatchConnectorFetchByUserSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByUser("user2", s.time, 1, true)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("user2", patches[0].Author)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooFewAsc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time, 1, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*4), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchTooFewDesc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Second*10), 1, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchNonexistentFail() {
	patches, err := s.ctx.FindPatchesByUser("zzz", s.time, 1, true)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyWithinBoundAsc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Second*4), 1, true)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyWithinBoundDesc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Second*6), 1, false)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*6), patches[0].CreateTime)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyOutOfBoundAsc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(time.Hour), 1, true)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorFetchByUserSuite) TestFetchKeyOutOfBoundDesc() {
	patches, err := s.ctx.FindPatchesByUser("user1", s.time.Add(-time.Hour), 1, false)
	s.NoError(err)
	s.Len(patches, 0)
}
