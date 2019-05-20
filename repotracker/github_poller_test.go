package repotracker

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/jasper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

var (
	testConfig            = testutil.TestConfig()
	firstRevision         = "99162ee5bc41eb314f5bb01bd12f0c43e9cb5f32"
	lastRevision          = "d0d878e81b303fd2abbf09331e54af41d6cd0c7d"
	firstRemoteConfigRef  = "6dbe53d948906ed3e0a355eb25b9d54e5b011209"
	secondRemoteConfigRef = "9b6c7d7f479da84b767995076b13c31796a5e2bf"
	badRemoteConfigRef    = "276382eb9f5ebcfce2791d1c99ce5e591023146b"

	projectRef    *model.ProjectRef
	evgProjectRef *model.ProjectRef
)

type bufCloser struct {
	*bytes.Buffer
}

func (b *bufCloser) Close() error { return nil }

func getDistantEVGRevision() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	buf := &bufCloser{&bytes.Buffer{}}

	cmd := jasper.NewCommand().Add([]string{"git", "rev-list", "--reverse", "HEAD~100", "--max-count=1"}).Directory(cwd).
		SuppressStandardError(true).SetOutputWriter(buf)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = cmd.Run(ctx)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
}

func resetProjectRefs() {
	projectRef = &model.ProjectRef{
		Identifier:  "mci-test",
		DisplayName: "MCI Test",
		Owner:       "deafgoat",
		Repo:        "mci-test",
		Branch:      "master",
		RepoKind:    "github",
		RemotePath:  "mci",
		Enabled:     true,
		Private:     false,
		BatchTime:   60,
		Tracked:     true,
	}
	evgProjectRef = &model.ProjectRef{
		Repo:       "evergreen",
		Owner:      "evergreen-ci",
		Identifier: "mci",
		Branch:     "master",
		RemotePath: "self-tests.yml",
		RepoKind:   "github",
		Enabled:    true,
	}
}

func init() {
	resetProjectRefs()
}

func dropTestDB(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, err, "Error opening database session")
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase(),
		"Error dropping test database")
	createTaskCollections()
}

func createTaskCollections() {
	cmd := map[string]string{
		"create": task.Collection,
	}
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
	cmd["create"] = build.Collection
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
	cmd["create"] = model.VersionCollection
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
}

func TestGetRevisionsSinceWithPaging(t *testing.T) {
	dropTestDB(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetRevisionsSince")
	Convey("When fetching commits from the evergreen repository", t, func() {
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)
		grp := &GithubRepositoryPoller{
			ProjectRef: evgProjectRef,
			OauthToken: token,
		}
		Convey("fetching > the size of a github page should succeed", func() {
			distantEvgRevision, err := getDistantEVGRevision()
			So(err, ShouldBeNil)
			So(distantEvgRevision, ShouldNotEqual, "")
			revisions, err := grp.GetRevisionsSince(distantEvgRevision, 5000)
			So(err, ShouldBeNil)
			Convey("and the revision should be found", func() {
				So(len(revisions), ShouldNotEqual, 0)
			})
		})
	})
}

func TestGetRevisionsSince(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetRevisionsSince")

	Convey("When fetching github revisions (by commit) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		self.ProjectRef = projectRef
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)
		self.OauthToken = token

		Convey("There should be only two revisions since the first revision",
			func() {
				// The test repository contains only 3 revisions with revision
				// 99162ee5bc41eb314f5bb01bd12f0c43e9cb5f32 being the first
				// revision
				revisions, err := self.GetRevisionsSince(firstRevision, 10)
				require.NoError(t, err, "Error fetching github revisions")
				So(len(revisions), ShouldEqual, 2)

				// Friday, February 15, 2008 2:59:14 PM GMT-05:00
				minTime := time.Unix(1203105554, 0)

				Convey("date for author commit should never be prior to 2008", func() {
					for _, revision := range revisions {
						So(util.IsZeroTime(revision.CreateTime), ShouldBeFalse)
						So(revision.CreateTime.After(minTime), ShouldBeTrue)
					}
				})
			})

		Convey("There should be no revisions since the last revision", func() {
			// The test repository contains only 3 revisions with revision
			// d0d878e81b303fd2abbf09331e54af41d6cd0c7d being the last revision
			revisions, err := self.GetRevisionsSince(lastRevision, 10)
			require.NoError(t, err, "Error fetching github revisions")
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
		Convey("If the revision is not valid because it has less than 10 characters, should return an error", func() {
			_, err := self.GetRevisionsSince("master", 10)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGetRemoteConfig(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetRemoteConfig")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When fetching a specific github revision configuration...",
		t, func() {

			self.ProjectRef = &model.ProjectRef{
				Identifier:  "mci-test",
				DisplayName: "MCI Test",
				Owner:       "deafgoat",
				Repo:        "config",
				Branch:      "master",
				RepoKind:    "github",
				RemotePath:  "random.txt",
				Enabled:     true,
				Private:     false,
				BatchTime:   60,
				Tracked:     true,
			}
			token, err := testConfig.GetGithubOauthToken()
			So(err, ShouldBeNil)
			self.OauthToken = token

			Convey("The config file at the requested revision should be "+
				"exactly what is returned", func() {
				projectConfig, err := self.GetRemoteConfig(ctx, firstRemoteConfigRef)
				require.NoError(t, err, "Error fetching github "+
					"configuration file")
				So(projectConfig, ShouldNotBeNil)
				So(len(projectConfig.Tasks), ShouldEqual, 0)
				projectConfig, err = self.GetRemoteConfig(ctx, secondRemoteConfigRef)
				require.NoError(t, err, "Error fetching github "+
					"configuration file")
				So(projectConfig, ShouldNotBeNil)
				So(len(projectConfig.Tasks), ShouldEqual, 1)
			})
			Convey("an invalid revision should return an error", func() {
				_, err := self.GetRemoteConfig(ctx, "firstRemoteConfRef")
				So(err, ShouldNotBeNil)
			})
			Convey("an invalid project configuration should error out", func() {
				_, err := self.GetRemoteConfig(ctx, badRemoteConfigRef)
				So(err, ShouldNotBeNil)
			})
		})
}

func TestGetAllRevisions(t *testing.T) {
	dropTestDB(t)
	var self GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetAllRevisions")

	Convey("When fetching recent github revisions (by count) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		self.ProjectRef = projectRef
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)
		self.OauthToken = token

		// Even though we're requesting far more revisions than exists in the
		// remote repository, we should only get the revisions that actually
		// exist upstream - a total of 3
		Convey("There should be only three revisions even if you request more "+
			"than 3", func() {
			revisions, err := self.GetRecentRevisions(123)
			require.NoError(t, err, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 3)
		})

		// Get only one recent revision and ensure it's the right revision
		Convey("There should be only be one if you request 1 and it should be "+
			"the latest", func() {
			revisions, err := self.GetRecentRevisions(1)
			require.NoError(t, err, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 1)
			So(revisions[0].Revision, ShouldEqual, lastRevision)
		})

		// Get no recent revisions
		Convey("There should be no revisions if you request 0", func() {
			revisions, err := self.GetRecentRevisions(0)
			require.NoError(t, err, "Error fetching github revisions")
			So(len(revisions), ShouldEqual, 0)
		})
	})
}

func TestGetChangedFiles(t *testing.T) {
	dropTestDB(t)
	var grp GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetAllRevisions")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When fetching changed files from evergreen-ci/evergreen ", t, func() {
		grp.ProjectRef = evgProjectRef
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)
		grp.OauthToken = token

		r1 := "b11fcb25624c6a0649dd35b895f5b550d649a128"
		Convey("the revision "+r1+" should have 8 files", func() {
			files, err := grp.GetChangedFiles(ctx, r1)
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 8)
			So(files, ShouldContain, "cloud/providers/ec2/ec2.go")
			So(files, ShouldContain, "public/static/dist/css/styles.css")
			// ...probably don't need to check all 8
		})

		r2 := "3c0133dbd4b35418c11df7b6e3a1ae31f966de42"
		Convey("the revision "+r2+" should have 1 file", func() {
			files, err := grp.GetChangedFiles(ctx, r2)
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 1)
			So(files, ShouldContain, "ui/rest_project.go")
		})

		Convey("a revision that does not exist should fail", func() {
			files, err := grp.GetChangedFiles(ctx, "00000000000000000000000000")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "No commit found for SHA: 00000000000000000000000000")
			So(files, ShouldBeNil)
		})
	})
}

func TestIsLastRevision(t *testing.T) {
	Convey("When calling isLastRevision...", t, func() {
		Convey("it should not panic on nil SHA hash", func() {
			githubCommit := &github.RepositoryCommit{}
			So(isLastRevision(firstRevision, githubCommit), ShouldBeFalse)
		})
		Convey("it should return false if the commit SHA does not match "+
			"the revision string passed in", func() {
			githubCommit := &github.RepositoryCommit{
				SHA: github.String("someotherhash"),
			}
			So(isLastRevision(firstRevision, githubCommit), ShouldBeFalse)
		})
		Convey("it should return true if the commit SHA matches "+
			"the revision string passed in", func() {
			githubCommit := &github.RepositoryCommit{
				SHA: github.String(firstRevision),
			}
			So(isLastRevision(firstRevision, githubCommit), ShouldBeTrue)
		})
	})
}
