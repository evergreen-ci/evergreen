package service

import (
	"fmt"
	"testing"

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
	radius         int
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
	for i := 0; i < 25; i++ {
		version := model.Version{
			Id:                  fmt.Sprintf("version_%d", i),
			RevisionOrderNumber: i + 1,
			Identifier:          s.projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		s.versionsBefore = append([]model.Version{version}, s.versionsBefore...)
		s.NoError(version.Insert())
	}

	s.middleVersion = model.Version{
		Id:                  "middle",
		RevisionOrderNumber: 26,
		Identifier:          s.projectID,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	s.NoError(s.middleVersion.Insert())

	for i := 26; i < 51; i++ {
		version := model.Version{
			Id:                  fmt.Sprintf("version_%d", i),
			RevisionOrderNumber: i + 1,
			Identifier:          s.projectID,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		s.versionsAfter = append([]model.Version{version}, s.versionsAfter...)
		s.NoError(version.Insert())
	}

	s.radius = 25
}

func (s *TaskHistorySuite) TestGetVersionsInWindow() {
	versions, err := getVersionsInWindow("surround", s.projectID, s.radius, &s.middleVersion)
	s.Require().NoError(err)
	s.Require().Len(versions, (2*s.radius)+1)

	for i := 0; i < s.radius; i++ {
		s.Equal(s.versionsAfter[i].Id, versions[i].Id)
	}

	s.Equal(s.middleVersion.Id, versions[s.radius].Id)

	for i := 0; i < s.radius; i++ {
		s.Equal(s.versionsBefore[i].Id, versions[i+(s.radius+1)].Id)
	}
}

func (s *TaskHistorySuite) TestSurroundingVersions() {
	versionsAfter, err := surroundingVersions(&s.middleVersion, s.projectID, s.radius, false)
	s.NoError(err)
	s.Require().Len(versionsAfter, s.radius)
	for i, version := range versionsAfter {
		s.Equal(s.versionsAfter[i].Id, version.Id)
	}

	versionsBefore, err := surroundingVersions(&s.middleVersion, s.projectID, s.radius, true)
	s.NoError(err)
	s.Len(versionsBefore, s.radius)
	for i, version := range versionsBefore {
		s.Equal(s.versionsBefore[i].Id, version.Id)
	}
}
