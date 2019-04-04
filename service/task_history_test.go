package service

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
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
			Id:         util.RandomString(),
			Revision:   "cccc",
			CreateTime: now,
			Identifier: s.projectID,
			Requester:  evergreen.RepotrackerVersionRequester,
		}
		s.versionsAfter = append(s.versionsAfter, version)
		s.NoError(version.Insert())
	}

	// Middle version with the same createTime as the last version in versionsBefore
	// and the first version in versionsAfter
	s.middleVersion = model.Version{
		Id:         util.RandomString(),
		Revision:   "bbbb",
		CreateTime: now,
		Identifier: s.projectID,
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	s.NoError(s.middleVersion.Insert())

	for i := 0; i < 50; i++ {
		version := model.Version{
			Id:         util.RandomString(),
			Revision:   "aaaa",
			CreateTime: now,
			Identifier: s.projectID,
			Requester:  evergreen.RepotrackerVersionRequester,
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
		s.Equal("cccc", version.Revision)
		// compare to the last _radius_ elements in s.versionsAfter
		s.True(version.CreateTime.Equal(s.versionsAfter[(len(s.versionsAfter)-radius)+i].CreateTime))
	}

	versionsBefore, err := surroundingVersions(&s.middleVersion, s.projectID, radius, true)
	s.NoError(err)
	s.Len(versionsBefore, radius)
	for i, version := range versionsBefore {
		s.Equal("aaaa", version.Revision)
		s.True(version.CreateTime.Equal(s.versionsBefore[i].CreateTime))
	}
}
