package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/suite"
)

type TaskHistorySuite struct {
	suite.Suite
	versionsBefore []model.Version
	middleVersion  model.Version
	versionsAfter  []model.Version
	projectID      string
}

func TestTaskHistory(t *testing.T) {
	suite.Run(t, new(TaskHistorySuite))
}

func (s *TaskHistorySuite) SetupSuite() {
	s.projectID = "mci"
}

func (s *TaskHistorySuite) SetupTest() {
	s.NoError(db.ClearCollections(model.VersionCollection))
	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	for i := 0; i < 50; i++ {
		// backwards in time, in descending order
		now = now.Add(-time.Minute)
		version := model.Version{
			Id:                  fmt.Sprintf("after_%d", i),
			RevisionOrderNumber: 101 - i,
			CreateTime:          now,
			Identifier:          s.projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		s.versionsAfter = append(s.versionsAfter, version)
		s.NoError(version.Insert())
	}

	// Middle version with the same createTime as the last version in versionsBefore
	// and the first version in versionsAfter
	s.middleVersion = model.Version{
		Id:                  "middle",
		RevisionOrderNumber: 51,
		CreateTime:          now,
		Identifier:          s.projectID,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	s.NoError(s.middleVersion.Insert())

	for i := 0; i < 50; i++ {
		version := model.Version{
			Id:                  fmt.Sprintf("before_%d", i),
			RevisionOrderNumber: 50 - i,
			CreateTime:          now,
			Identifier:          s.projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		s.versionsBefore = append(s.versionsBefore, version)
		s.NoError(version.Insert())

		now = now.Add(-time.Minute)
	}
}

func (s *TaskHistorySuite) TestGetVersionsInWindow() {
	radius := 20
	versions, err := getVersionsInWindow("surround", s.projectID, radius, &s.middleVersion)
	s.Require().NoError(err)

	for i := 0; i < radius; i++ {
		// compare to the last _radius_ elements in s.versionsAfter
		s.True(versions[i].CreateTime.Equal(s.versionsAfter[(len(s.versionsAfter))-radius+i].CreateTime))
	}

	s.True(s.middleVersion.CreateTime.Equal(versions[radius].CreateTime))

	for i := 0; i < radius; i++ {
		s.True(versions[i+(1+radius)].CreateTime.Equal(s.versionsBefore[i].CreateTime))
	}
}

func (s *TaskHistorySuite) TestSurroundingVersions() {
	radius := 20
	versionsAfter, err := surroundingVersions(&s.middleVersion, s.projectID, radius, false)
	s.NoError(err)
	s.Require().Len(versionsAfter, radius)
	for i, version := range versionsAfter {
		s.True(version.RevisionOrderNumber > s.middleVersion.RevisionOrderNumber)
		// compare to the last _radius_ elements in s.versionsAfter
		s.True(version.CreateTime.Equal(s.versionsAfter[(len(s.versionsAfter)-radius)+i].CreateTime))
	}

	versionsBefore, err := surroundingVersions(&s.middleVersion, s.projectID, radius, true)
	s.NoError(err)
	s.Len(versionsBefore, radius)
	for i, version := range versionsBefore {
		s.True(version.RevisionOrderNumber < s.middleVersion.RevisionOrderNumber)
		s.True(version.CreateTime.Equal(s.versionsBefore[i].CreateTime))
	}
}

func (s *TaskHistorySuite) TestSurroundingVersionsSort() {
	versionsAfter, err := surroundingVersions(&s.versionsBefore[0], s.projectID, 2, false)
	s.NoError(err)
	s.Require().Len(versionsAfter, 2)
	// sorted ascending, then reversed
	s.Equal(52, versionsAfter[0].RevisionOrderNumber)
	s.Equal("after_49", versionsAfter[0].Id)
	s.Equal(51, versionsAfter[1].RevisionOrderNumber)
	s.Equal("middle", versionsAfter[1].Id)

	versionsBefore, err := surroundingVersions(&s.versionsAfter[49], s.projectID, 2, true)
	s.NoError(err)
	s.Require().Len(versionsBefore, 2)
	// sorted descending
	s.Equal(51, versionsBefore[0].RevisionOrderNumber)
	s.Equal("middle", versionsBefore[0].Id)
	s.Equal(50, versionsBefore[1].RevisionOrderNumber)
	s.Equal("before_0", versionsBefore[1].Id)
}
