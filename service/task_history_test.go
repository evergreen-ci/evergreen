package service

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
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
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.projectID = "mci"
}

func (s *TaskHistorySuite) SetupTest() {
	s.NoError(db.ClearCollections(model.VersionCollection))
	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	for i := 0; i < 50; i++ {
		now = now.Add(time.Minute)
		version := model.Version{
			Id:         util.RandomString(),
			Revision:   "aaaa",
			CreateTime: now,
			Identifier: s.projectID,
			Requester:  evergreen.RepotrackerVersionRequester,
		}
		s.versionsBefore = append(s.versionsBefore, version)
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
			Revision:   "cccc",
			CreateTime: now,
			Identifier: s.projectID,
			Requester:  evergreen.RepotrackerVersionRequester,
		}
		s.versionsAfter = append(s.versionsAfter, version)
		s.NoError(version.Insert())

		now = now.Add(time.Minute)
	}
}

func (s *TaskHistorySuite) TestGetVersionsInWindow() {
	radius := 20
	versions, err := getVersionsInWindow("surround", s.projectID, radius, &s.middleVersion)
	s.NoError(err)

	for i := 0; i < radius; i++ {
		s.True(versions[i].CreateTime.Equal(s.versionsAfter[(radius-1)-i].CreateTime))
	}

	s.True(s.middleVersion.CreateTime.Equal(versions[radius].CreateTime))

	for i := 0; i < radius; i++ {
		s.True(versions[i+(1+radius)].CreateTime.Equal(s.versionsBefore[(len(s.versionsBefore)-1)-i].CreateTime))
	}
}

func (s *TaskHistorySuite) TestMakeVersionsQuery() {
	versionsAfter, err := makeVersionsQuery(&s.middleVersion, s.projectID, 20, false)
	s.NoError(err)
	s.Len(versionsAfter, 20)
	for i, version := range versionsAfter {
		s.Equal("cccc", version.Revision)
		s.True(version.CreateTime.Equal(s.versionsAfter[i].CreateTime))
	}

	versionsBefore, err := makeVersionsQuery(&s.middleVersion, s.projectID, 20, true)
	s.NoError(err)
	s.Len(versionsBefore, 20)
	for i, version := range versionsBefore {
		s.Equal("aaaa", version.Revision)
		s.True(version.CreateTime.Equal(s.versionsBefore[len(s.versionsBefore)-(i+1)].CreateTime))
	}
}
