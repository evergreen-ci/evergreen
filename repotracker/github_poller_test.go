package repotracker

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/testutils"
	"10gen.com/mci/thirdparty"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	testConfig            = mci.TestConfig()
	firstRevision         = "99162ee5bc41eb314f5bb01bd12f0c43e9cb5f32"
	lastRevision          = "d0d878e81b303fd2abbf09331e54af41d6cd0c7d"
	firstRemoteConfigRef  = "6dbe53d948906ed3e0a355eb25b9d54e5b011209"
	secondRemoteConfigRef = "9b6c7d7f479da84b767995076b13c31796a5e2bf"
	badRemoteConfigRef    = "276382eb9f5ebcfce2791d1c99ce5e591023146b"
	projectRef            = NewProjectRef("deafgoat", "mci-test",
		"master", "github", "mci", true, false, false, 60, "mci-test",
		"MCI Test", true)
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	if testConfig.RepoTracker.LogFile != "" {
		mci.SetLogger(testConfig.RepoTracker.LogFile)
	}
}

func dropTestDB(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "Error opening database session")
	defer session.Close()
	util.HandleTestingErr(session.DB(testConfig.Db).DropDatabase(), t, "Error "+
		"dropping test database")
}

func TestGetRevisionsSince(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutils.ConfigureIntegrationTest(t, testConfig, "TestGetRevisionsSince")

	Convey("When fetching github revisions (by commit) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		self.ProjectRef = projectRef
		self.OauthToken = testConfig.Credentials[self.ProjectRef.RepoKind]

		Convey("There should be only two revisions since the first revision",
			func() {
				// The test repository contains only 3 revisions with revision
				// 99162ee5bc41eb314f5bb01bd12f0c43e9cb5f32 being the first
				// revision
				revisions, err := self.GetRevisionsSince(firstRevision, 10)
				util.HandleTestingErr(err, t, "Error fetching github revisions")
				So(len(revisions), ShouldEqual, 2)
			})

		Convey("There should be no revisions since the last revision", func() {
			// The test repository contains only 3 revisions with revision
			// d0d878e81b303fd2abbf09331e54af41d6cd0c7d being the last revision
			revisions, err := self.GetRevisionsSince(lastRevision, 10)
			util.HandleTestingErr(err, t, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 0)
		})

		Convey("There should be an error returned if the requested revision "+
			"isn't found", func() {
			// The test repository contains only 3 revisions with revision
			// d0d878e81b303fd2abbf09331e54af41d6cd0c7d being the last revision
			revisions, err := self.GetRevisionsSince("lastRevision", 10)
			So(len(revisions), ShouldEqual, 0)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGetRemoteConfig(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutils.ConfigureIntegrationTest(t, testConfig, "TestGetRemoteConfig")

	Convey("When fetching a specific github revision configuration...",
		t, func() {
			self.ProjectRef = NewProjectRef("deafgoat", "config",
				"master", "github", "random.txt", true, false, true, 60,
				"repotracer", "repotracker", true)
			self.OauthToken = testConfig.Credentials[self.ProjectRef.RepoKind]

			Convey("The config file at the requested revision should be "+
				"exactly what is returned", func() {
				projectConfig, err := self.GetRemoteConfig(firstRemoteConfigRef)
				util.HandleTestingErr(err, t, "Error fetching github "+
					"configuration file")
				So(projectConfig, ShouldNotBeNil)
				So(projectConfig.Owner, ShouldEqual, "deafgoat")
				So(len(projectConfig.Tasks), ShouldEqual, 0)
				projectConfig, err = self.GetRemoteConfig(secondRemoteConfigRef)
				util.HandleTestingErr(err, t, "Error fetching github "+
					"configuration file")
				So(projectConfig, ShouldNotBeNil)
				So(projectConfig.Owner, ShouldEqual, "deafgoat")
				So(len(projectConfig.Tasks), ShouldEqual, 1)
			})
			Convey("an invalid revision should return an error", func() {
				_, err := self.GetRemoteConfig("firstRemoteConfRef")
				So(err, ShouldNotBeNil)
			})
			Convey("an invalid project configuration should error out", func() {
				_, err := self.GetRemoteConfig(badRemoteConfigRef)
				So(err, ShouldNotBeNil)
			})
		})
}

func TestGetAllRevisions(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutils.ConfigureIntegrationTest(t, testConfig, "TestGetAllRevisions")

	Convey("When fetching recent github revisions (by count) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		self.ProjectRef = projectRef
		self.OauthToken = testConfig.Credentials[self.ProjectRef.RepoKind]

		// Even though we're requesting far more revisions than exists in the
		// remote repository, we should only get the revisions that actually
		// exist upstream - a total of 3
		Convey("There should be only three revisions even if you request more "+
			"than 3", func() {
			revisions, err := self.GetRecentRevisions(123)
			util.HandleTestingErr(err, t, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 3)
		})

		// Get only one recent revision and ensure it's the right revision
		Convey("There should be only be one if you request 1 and it should be "+
			"the latest", func() {
			revisions, err := self.GetRecentRevisions(1)
			util.HandleTestingErr(err, t, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 1)
			So(revisions[0].Revision, ShouldEqual, lastRevision)
		})

		// Get no recent revisions
		Convey("There should be no revisions if you request 0", func() {
			revisions, err := self.GetRecentRevisions(0)
			util.HandleTestingErr(err, t, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 0)
		})
	})
}

func TestIsLastRevision(t *testing.T) {
	Convey("When calling isLastRevision...", t, func() {
		Convey("it should return false if the commit SHA does not match "+
			"the revision string passed in", func() {
			githubCommit := &thirdparty.GithubCommit{}
			So(isLastRevision(firstRevision, githubCommit), ShouldBeFalse)
		})
		Convey("it should return true if the commit SHA matches "+
			"the revision string passed in", func() {
			githubCommit := &thirdparty.GithubCommit{}
			githubCommit.SHA = firstRevision
			So(isLastRevision(firstRevision, githubCommit), ShouldBeTrue)
		})
	})
}

func TestGetCommitURL(t *testing.T) {
	Convey("When calling getCommitURL...", t, func() {
		Convey("the returned string should use the fields of the project "+
			"ref correctly", func() {
			projectRef := &model.ProjectRef{
				Owner:  "a",
				Repo:   "b",
				Branch: "c",
			}
			expectedURL := "https://api.github.com/repos/a/b/commits?sha=c"
			So(getCommitURL(projectRef), ShouldEqual, expectedURL)
		})
	})
}
