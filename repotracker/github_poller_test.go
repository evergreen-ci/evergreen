package repotracker

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testConfig     = testutil.TestConfig()
	firstRevision  = "15d88cbfd0b5196eebdfbeecc404e04a5b8a4507"
	secondRevision = "94bbeb8593ffa9f7dc702a4ab0cc12283dbd3233"
	lastRevision   = "e82137ae00cc85692175b63e008261435bda7cb3"
	badRevision    = "276382eb9f5ebcfce2791d1c99ce5e591023146b"

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
	bufErr := &bufCloser{&bytes.Buffer{}}

	cmd := jasper.NewCommand().Add([]string{"git", "rev-list", "--reverse", "--max-count=1", "HEAD~100"}).Directory(cwd).
		SetOutputWriter(buf).SetErrorWriter(bufErr)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = cmd.Run(ctx)
	if err != nil {
		return "", errors.Wrap(err, bufErr.String())
	}

	return strings.TrimSpace(buf.String()), nil
}

func resetProjectRefs() {
	// TODO: should use an evergreen-owned repo for the integration tests.
	projectRef = &model.ProjectRef{
		Id:          "mci-test",
		DisplayName: "MCI Test",
		Owner:       "evergreen-ci",
		Repo:        "integration-tests",
		Branch:      "main",
		RemotePath:  "commit.txt",
		Enabled:     true,
		BatchTime:   60,
		Hidden:      utility.FalsePtr(),
	}
	evgProjectRef = &model.ProjectRef{
		Repo:       "evergreen",
		Owner:      "evergreen-ci",
		Id:         "mci",
		Branch:     "main",
		RemotePath: "self-tests.yml",
		Enabled:    true,
	}
}

func init() {
	resetProjectRefs()
}

func dropTestDB(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	createTaskCollections(t)
}

func createTaskCollections(t *testing.T) {
	require.NoError(t, db.CreateCollections(task.Collection, build.Collection, model.VersionCollection, model.ParserProjectCollection, model.ProjectConfigCollection))
}

func TestGetRevisionsSinceWithPaging(t *testing.T) {
	dropTestDB(t)
	testutil.ConfigureIntegrationTest(t, testConfig)
	Convey("When fetching commits from the evergreen repository", t, func() {
		grp := &GithubRepositoryPoller{
			ProjectRef: evgProjectRef,
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
	var ghp GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig)

	// Initialize repo revisions for project
	_, err := model.GetNewRevisionOrderNumber(projectRef.Id)
	require.NoError(t, err)

	// The test repository contains only 3 revisions.
	Convey("When fetching github revisions (by commit) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		ghp.ProjectRef = projectRef

		Convey("There should be only two revisions since the first revision",
			func() {
				revisions, err := ghp.GetRevisionsSince(firstRevision, 10)
				require.NoError(t, err)
				So(len(revisions), ShouldEqual, 2)

				// Friday, February 15, 2008 2:59:14 PM GMT-05:00
				minTime := time.Unix(1203105554, 0)

				Convey("date for author commit should never be prior to 2008", func() {
					for _, revision := range revisions {
						So(utility.IsZeroTime(revision.CreateTime), ShouldBeFalse)
						So(revision.CreateTime.After(minTime), ShouldBeTrue)
					}
				})
			})

		Convey("There should be no revisions since the last revision", func() {
			revisions, err := ghp.GetRevisionsSince(lastRevision, 10)
			require.NoError(t, err)
			So(len(revisions), ShouldEqual, 0)
		})

		Convey("There should be an error returned if the requested revision "+
			"isn't found", func() {
			revisions, err := ghp.GetRevisionsSince(badRevision, 10)
			So(len(revisions), ShouldEqual, 0)
			So(err, ShouldNotBeNil)
		})
		Convey("If the revision is not valid because it has less than 10 characters, should return an error", func() {
			_, err := ghp.GetRevisionsSince("master", 10)
			So(err, ShouldNotBeNil)
		})
		Convey("should limit number of revisions returned and set last repo revision", func() {
			const maxRevisions = 1
			// There are supposed to be 3 revisions in the repo in total, so given the maxRevisions limit, the first
			// (i.e. earliest) revisions should not be found. The expected behavior here is that if it cannot find the
			// given revision, it will search for maxRevisions, then add one revision as the new base revision if
			// necessary.
			revisions, err := ghp.GetRevisionsSince(firstRevision, maxRevisions)
			require.NoError(t, err)
			require.Len(t, revisions, maxRevisions+1, "should find revisions up to limit, plus one for the base revision when the starting revision is not found in the listed revisions")
			assert.Equal(t, lastRevision, revisions[0].Revision, "revisions should be reverse time ordered")
			lastRepoRevision, err := model.FindRepository(projectRef.Id)
			require.NoError(t, err)
			assert.Equal(t, firstRevision, lastRepoRevision.LastRevision, "last revision should be updated to base revision when the starting revision is not found in listed revisions")
		})
	})
}

func TestGetRemoteConfig(t *testing.T) {
	dropTestDB(t)
	var ghp GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When fetching a specific github revision configuration...",
		t, func() {

			ghp.ProjectRef = &model.ProjectRef{
				Id:          "mci-test",
				DisplayName: "MCI Test",
				Owner:       "evergreen-ci",
				Repo:        "integration-tests",
				Branch:      "main",
				RemotePath:  "tests.yml",
				Enabled:     true,
				BatchTime:   60,
				Hidden:      utility.FalsePtr(),
			}

			Convey("The config file at the requested revision should be "+
				"exactly what is returned", func() {
				projectInfo, err := ghp.GetRemoteConfig(ctx, firstRevision)
				require.NoError(t, err, "Error fetching github "+
					"configuration file")
				So(projectInfo.Project, ShouldNotBeNil)
				So(len(projectInfo.Project.Tasks), ShouldEqual, 0)
				projectInfo, err = ghp.GetRemoteConfig(ctx, secondRevision)
				require.NoError(t, err, "Error fetching github "+
					"configuration file")
				So(projectInfo.Project, ShouldNotBeNil)
				So(len(projectInfo.Project.Tasks), ShouldEqual, 1)
			})
			Convey("an invalid revision should return an error", func() {
				_, err := ghp.GetRemoteConfig(ctx, badRevision)
				So(err, ShouldNotBeNil)
			})
			Convey("an invalid project configuration should error out", func() {
				_, err := ghp.GetRemoteConfig(ctx, lastRevision)
				So(err, ShouldNotBeNil)
			})
		})
}

func TestGetAllRevisions(t *testing.T) {
	dropTestDB(t)
	var ghp GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig)

	Convey("When fetching recent github revisions (by count) - from a repo "+
		"containing 3 commits - given a valid Oauth token...", t, func() {
		ghp.ProjectRef = projectRef

		// Even though we're requesting far more revisions than exists in the
		// remote repository, we should only get the revisions that actually
		// exist upstream - a total of 3
		Convey("There should be only three revisions even if you request more "+
			"than 3", func() {
			revisions, err := ghp.GetRecentRevisions(123)
			require.NoError(t, err)
			So(len(revisions), ShouldEqual, 3)
		})

		// Get only one recent revision and ensure it's the right revision
		Convey("There should be only be one if you request 1 and it should be "+
			"the latest", func() {
			revisions, err := ghp.GetRecentRevisions(1)
			require.NoError(t, err)
			So(len(revisions), ShouldEqual, 1)
			So(revisions[0].Revision, ShouldEqual, lastRevision)
		})

		// Get no recent revisions
		Convey("There should be no revisions if you request 0", func() {
			revisions, err := ghp.GetRecentRevisions(0)
			require.NoError(t, err)
			So(len(revisions), ShouldEqual, 0)
		})
	})
}

func TestGetChangedFiles(t *testing.T) {
	dropTestDB(t)
	var grp GithubRepositoryPoller

	testutil.ConfigureIntegrationTest(t, testConfig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When fetching changed files from evergreen-ci/evergreen ", t, func() {
		grp.ProjectRef = evgProjectRef

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
