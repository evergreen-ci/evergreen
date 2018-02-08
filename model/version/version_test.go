package version

import (
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type testFindDuplicateVersionsSuite struct {
	suite.Suite
}

func TestFindDuplicateVersions(t *testing.T) {
	suite.Run(t, new(testFindDuplicateVersionsSuite))
}

func (s *testFindDuplicateVersionsSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func (s *testFindDuplicateVersionsSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection))

	versions := []Version{
		{
			Id:         "project_-1",
			Identifier: "project",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Time{}.Add(-time.Hour),
		},
		{
			Id:         "project_0",
			Identifier: "project",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project_patch_0",
			Identifier: "project",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.PatchVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project_patch_1",
			Identifier: "project",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.PatchVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project_1",
			Identifier: "project",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project2_0",
			Identifier: "project2",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project2_1",
			Identifier: "project2",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Now(),
		},
		{
			Id:         "project3_0",
			Identifier: "project3",
			Revision:   "96aeacfdb9667bb3905925768a1f240012d5735f",
			Requester:  evergreen.RepotrackerVersionRequester,
			CreateTime: time.Now(),
		},
	}
	for _, v := range versions {
		s.NoError(v.Insert())
	}
}

func (s *testFindDuplicateVersionsSuite) TestIt() {
	versions, err := FindDuplicateVersions(time.Time{})
	s.NoError(err)
	s.Len(versions, 2)

	for _, dv := range versions {
		if dv.ID.ProjectID == "project2" {
			s.Equal("96aeacfdb9667bb3905925768a1f240012d5735f", dv.ID.Hash)
			for _, v := range dv.Versions {
				s.Equal(evergreen.RepotrackerVersionRequester, v.Requester)
				s.True(v.CreateTime.After(time.Time{}))
				s.True(strings.HasPrefix(v.Id, "project2_"))
			}

		} else if dv.ID.ProjectID == "project" {
			s.Equal("96aeacfdb9667bb3905925768a1f240012d5735f", dv.ID.Hash)
			for _, v := range dv.Versions {
				s.Equal(evergreen.RepotrackerVersionRequester, v.Requester)
				s.True(v.CreateTime.After(time.Time{}))
				s.True(strings.HasPrefix(v.Id, "project_"))
			}

		} else {
			s.T().Fail()
		}
	}
}
