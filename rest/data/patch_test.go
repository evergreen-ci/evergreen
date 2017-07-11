package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type PatchConnectorSuite struct {
	ctx      Connector
	obj_ids  []bson.ObjectId
	time     time.Time
	setup    func() error
	teardown func() error
	suite.Suite
}

func TestPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
	s.setup = func() error {
		s.ctx = &DBConnector{}
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

		testutil.ConfigureIntegrationTest(t, testConfig, "TestPatchConnectorSuite")
		db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

		for i := 0; i < 6; i++ {
			s.obj_ids = append(s.obj_ids, bson.NewObjectId())
		}

		patches := []*patch.Patch{
			{Id: s.obj_ids[0], Project: "project1", CreateTime: s.time},
			{Id: s.obj_ids[1], Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
			{Id: s.obj_ids[2], Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
			{Id: s.obj_ids[3], Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
			{Id: s.obj_ids[4], Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
			{Id: s.obj_ids[5], Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
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

func TestMockPatchConnectorSuite(t *testing.T) {
	s := new(PatchConnectorSuite)
	s.setup = func() error {
		s.time = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.Local)

		s.obj_ids = []bson.ObjectId{}
		for i := 0; i < 6; i++ {
			s.obj_ids = append(s.obj_ids, bson.NewObjectId())
		}

		s.ctx = &MockConnector{MockPatchConnector: MockPatchConnector{
			CachedPatches: []patch.Patch{
				{Id: s.obj_ids[0], Project: "project1", CreateTime: s.time},
				{Id: s.obj_ids[1], Project: "project2", CreateTime: s.time.Add(time.Second * 2)},
				{Id: s.obj_ids[2], Project: "project1", CreateTime: s.time.Add(time.Second * 4)},
				{Id: s.obj_ids[3], Project: "project1", CreateTime: s.time.Add(time.Second * 6)},
				{Id: s.obj_ids[4], Project: "project2", CreateTime: s.time.Add(time.Second * 8)},
				{Id: s.obj_ids[5], Project: "project1", CreateTime: s.time.Add(time.Second * 10)},
			},
		}}

		return nil
	}

	s.teardown = func() error { return nil }

	suite.Run(t, s)
}

func (s *PatchConnectorSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *PatchConnectorSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *PatchConnectorSuite) TestFetchTooManyAsc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 3, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.Before(patches[1].CreateTime))
}

func (s *PatchConnectorSuite) TestFetchTooManyDesc() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time.Add(time.Hour), 3, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 2)
	s.Equal("project2", patches[0].Project)
	s.Equal("project2", patches[1].Project)
	s.True(patches[0].CreateTime.After(patches[1].CreateTime))
}

func (s *PatchConnectorSuite) TestFetchExactNumber() {
	patches, err := s.ctx.FindPatchesByProject("project2", s.time, 1, 1)
	s.NoError(err)
	s.NotNil(patches)

	s.Len(patches, 1)
	s.Equal("project2", patches[0].Project)
}

func (s *PatchConnectorSuite) TestFetchTooFewAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time, 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time, patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchTooFewDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Hour), 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*10), patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchNonexistentFail() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time, 1, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorSuite) TestFetchKeyWithinBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second), 1, 1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time.Add(time.Second*4), patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchKeyWithinBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project1", s.time.Add(time.Second), 1, -1)
	s.NoError(err)
	s.NotNil(patches)
	s.Len(patches, 1)
	s.Equal(s.time, patches[0].CreateTime)
}

func (s *PatchConnectorSuite) TestFetchKeyOutOfBoundAsc() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time.Add(time.Hour), 1, 1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorSuite) TestFetchKeyOutOfBoundDesc() {
	patches, err := s.ctx.FindPatchesByProject("project3", s.time, 1, -1)
	s.NoError(err)
	s.Len(patches, 0)
}

func (s *PatchConnectorSuite) TestFetchById() {
	p, err := s.ctx.FindPatchById(string(s.obj_ids[0]))
	s.NoError(err)
	s.NotNil(p)
	fmt.Println()
	s.Equal(string(s.obj_ids[0]), string(p.Id))
}

func (s *PatchConnectorSuite) TestFetchByIdFail() {
	new_id := bson.NewObjectId()
	for _, i := range s.obj_ids {
		s.NotEqual(new_id, i)
	}
	p, err := s.ctx.FindPatchById(string(new_id))
	s.Error(err)
	s.Nil(p)
}
