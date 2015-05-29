package repotracker

import (
	"errors"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutils"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
	"testing"
	"time"
)

var (
	_ fmt.Stringer = nil
)

type mockClock struct {
	FakeTime time.Time
}

func (c mockClock) Now() time.Time {
	return c.FakeTime
}

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	if testConfig.RepoTracker.LogFile != "" {
		evergreen.SetLogger(testConfig.RepoTracker.LogFile)
	}
}

func TestFetchRevisions(t *testing.T) {
	dropTestDB(t)
	testutils.ConfigureIntegrationTest(t, testConfig, "TestFetchRevisions")
	Convey("With a GithubRepositoryPoller with a valid OAuth token...", t, func() {
		err := testutils.CreateTestLocalConfig(testConfig, "mci-test")
		So(err, ShouldBeNil)
		repoTracker := RepoTracker{
			testConfig,
			projectRef,
			NewGithubRepositoryPoller(projectRef, testConfig.Credentials["github"]),
		}

		Convey("Fetching commits from the repository should not return "+
			"any errors", func() {
			So(repoTracker.FetchRevisions(10), ShouldBeNil)
		})

		Convey("Only get 3 revisions from the given repository if given a "+
			"limit of 4 commits where only 3 exist", func() {
			util.HandleTestingErr(repoTracker.FetchRevisions(4), t,
				"Error running repository process %v")
			numVersions, err := version.TotalVersions(bson.M{})
			util.HandleTestingErr(err, t, "Error finding all versions")
			So(numVersions, ShouldEqual, 3)
		})

		Convey("Only get 2 revisions from the given repository if given a "+
			"limit of 2 commits where 3 exist", func() {
			util.HandleTestingErr(repoTracker.FetchRevisions(2), t,
				"Error running repository process %v")
			numVersions, err := version.TotalVersions(bson.M{})
			util.HandleTestingErr(err, t, "Error finding all versions")
			So(numVersions, ShouldEqual, 2)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})
}

func TestStoreRepositoryRevisions(t *testing.T) {
	dropTestDB(t)
	testutils.ConfigureIntegrationTest(t, testConfig, "TestStoreRepositoryRevisions")
	Convey("When storing revisions gotten from a repository...", t, func() {
		err := testutils.CreateTestLocalConfig(testConfig, "mci-test")
		So(err, ShouldBeNil)
		repoTracker := RepoTracker{testConfig, projectRef, NewGithubRepositoryPoller(projectRef,
			testConfig.Credentials["github"])}

		// insert distros used in testing.
		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(), ShouldBeNil)

		Convey("On storing a single repo revision, we expect a version to be created"+
			" in the database for this project, which should be retrieved when we search"+
			" for this project's most recent version", func() {
			createTime := time.Now()
			revisionOne := *createTestRevision("firstRevision", createTime)
			revisions := []model.Revision{revisionOne}

			resultVersion, err := repoTracker.StoreRevisions(revisions)
			util.HandleTestingErr(err, t, "Error storing repository revisions %v")

			newestVersion, err := version.FindOne(version.ByMostRecentForRequester(projectRef.String(), evergreen.RepotrackerVersionRequester))
			util.HandleTestingErr(err, t, "Error retreiving newest version %v")

			So(resultVersion, ShouldResemble, newestVersion)
		})

		Convey("On storing several repo revisions, we expect a version to be created "+
			"for each revision", func() {
			createTime := time.Now()
			laterCreateTime := createTime.Add(time.Duration(4 * time.Hour))

			revisionOne := *createTestRevision("one", laterCreateTime)
			revisionTwo := *createTestRevision("two", createTime)

			revisions := []model.Revision{revisionOne, revisionTwo}

			_, err := repoTracker.StoreRevisions(revisions)
			util.HandleTestingErr(err, t, "Error storing repository revisions %v")

			versionOne, err := version.FindOne(version.ByProjectIdAndRevision(projectRef.Identifier, revisionOne.Revision))
			util.HandleTestingErr(err, t, "Error retrieving first stored version %v")
			versionTwo, err := version.FindOne(version.ByProjectIdAndRevision(projectRef.Identifier, revisionTwo.Revision))
			util.HandleTestingErr(err, t, "Error retreiving second stored version %v")

			So(versionOne.Revision, ShouldEqual, revisionOne.Revision)
			So(versionTwo.Revision, ShouldEqual, revisionTwo.Revision)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})

	Convey("When storing versions from repositories with remote configuration files...", t, func() {

		project := createTestProject(nil, nil)

		revisions := []model.Revision{
			*createTestRevision("foo", time.Now().Add(1*time.Minute)),
		}

		poller := NewMockRepoPoller(project, revisions)

		repoTracker := RepoTracker{
			testConfig,
			&model.ProjectRef{
				Identifier: "testproject",
				BatchTime:  10,
			},
			poller,
		}

		// insert distros used in testing.
		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(), ShouldBeNil)

		Convey("We should not fetch configs for versions we already have stored.",
			func() {
				So(poller.ConfigGets, ShouldBeZeroValue)
				// Store revisions the first time
				_, err := repoTracker.StoreRevisions(revisions)
				So(err, ShouldBeNil)
				// We should have fetched the config once for each revision
				So(poller.ConfigGets, ShouldEqual, len(revisions))

				// Store them again
				_, err = repoTracker.StoreRevisions(revisions)
				So(err, ShouldBeNil)
				// We shouldn't have fetched the config any additional times
				// since we have already stored these versions
				So(poller.ConfigGets, ShouldEqual, len(revisions))
			},
		)

		Convey("We should handle invalid configuration files gracefully by storing a stub version",
			func() {
				errStrs := []string{"Someone dun' goof'd"}
				poller.setNextError(projectConfigError{errStrs})
				stubVersion, err := repoTracker.StoreRevisions(revisions)
				// We want this error to get swallowed so a config error
				// doesn't stop additional versions from getting created
				So(err, ShouldBeNil)
				So(stubVersion.Errors, ShouldResemble, errStrs)
			},
		)

		Convey("If there is an error other than a config error while fetching a config, we should fail hard",
			func() {
				unexpectedError := errors.New("Something terrible has happened!!")
				poller.setNextError(unexpectedError)
				v, err := repoTracker.StoreRevisions(revisions)
				So(v, ShouldBeNil)
				So(err, ShouldEqual, unexpectedError)
			},
		)

		Reset(func() {
			dropTestDB(t)
		})

	})
}

func TestBatchTimes(t *testing.T) {
	dropTestDB(t)
	Convey("When deciding whether or not to activate variants for the most recently stored version", t, func() {
		// We create a version with an activation time of now so that all the bvs have a last activation time of now.
		previouslyActivatedVersion := version.Version{
			Id:      "previously activated",
			Project: "testproject",
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: "bv1",
					Activated:    true,
					ActivateAt:   time.Now(),
				},
				{
					BuildVariant: "bv2",
					Activated:    true,
					ActivateAt:   time.Now(),
				},
			},
			RevisionOrderNumber: 0,
			Requester:           evergreen.RepotrackerVersionRequester,
		}

		So(previouslyActivatedVersion.Insert(), ShouldBeNil)

		// insert distros used in testing.
		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(), ShouldBeNil)

		Convey("If the project's batch time has not elapsed, and no buildvariants "+
			"have overriden their batch times, no variants should be activated", func() {
			project := createTestProject(nil, nil)
			revisions := []model.Revision{
				*createTestRevision("foo", time.Now()),
			}

			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Identifier: "testproject",
					BatchTime:  1,
				},
				NewMockRepoPoller(project, revisions),
			}
			v, err := repoTracker.StoreRevisions(revisions)
			So(v, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(len(v.BuildVariants), ShouldEqual, 2)
			So(repoTracker.activateElapsedBuilds(v), ShouldBeNil)
			So(v.BuildVariants[0].Activated, ShouldBeFalse)
			So(v.BuildVariants[1].Activated, ShouldBeFalse)
		})

		Convey("If the project's batch time has elapsed, and no buildvariants "+
			"have overridden their batch times, all variants should be activated", func() {
			project := createTestProject(nil, nil)
			revisions := []model.Revision{
				*createTestRevision("bar", time.Now().Add(time.Duration(-6*time.Minute))),
			}
			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Identifier: "testproject",
					BatchTime:  0,
				},
				NewMockRepoPoller(project, revisions),
			}
			version, err := repoTracker.StoreRevisions(revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(repoTracker.activateElapsedBuilds(version), ShouldBeNil)
			bv1, found := findStatus(version, "bv1")
			So(found, ShouldBeTrue)
			So(bv1.Activated, ShouldBeTrue)
			bv2, found := findStatus(version, "bv2")
			So(found, ShouldBeTrue)
			So(bv2.Activated, ShouldBeTrue)
		})

		Convey("If the project's batch time has elapsed, but both variants "+
			"have overridden their batch times (which have not elapsed)"+
			", no variants should be activated", func() {
			// need to assign pointer vals
			twoforty := 240
			onetwenty := 120

			project := createTestProject(&twoforty, &onetwenty)

			revisions := []model.Revision{
				*createTestRevision("baz", time.Now()),
			}

			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Identifier: "testproject",
					BatchTime:  60,
				},
				NewMockRepoPoller(project, revisions),
			}
			version, err := repoTracker.StoreRevisions(revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(repoTracker.activateElapsedBuilds(version), ShouldBeNil)
			bv1, found := findStatus(version, "bv1")
			So(found, ShouldBeTrue)
			So(bv1.Activated, ShouldBeFalse)
			bv2, found := findStatus(version, "bv2")
			So(found, ShouldBeTrue)
			So(bv2.Activated, ShouldBeFalse)
		})

		Convey("If the project's batch time has not elapsed, but one variant "+
			"has overridden their batch times to be shorter"+
			", that variant should be activated", func() {
			zero := 0

			project := createTestProject(&zero, nil)

			revisions := []model.Revision{
				*createTestRevision("garply", time.Now()),
			}

			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Identifier: "testproject",
					BatchTime:  60,
				},
				NewMockRepoPoller(project, revisions),
			}
			version, err := repoTracker.StoreRevisions(revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(repoTracker.activateElapsedBuilds(version), ShouldBeNil)
			bv1, found := findStatus(version, "bv1")
			So(found, ShouldBeTrue)
			So(bv1.Activated, ShouldBeTrue)
			bv2, found := findStatus(version, "bv2")
			So(found, ShouldBeTrue)
			So(bv2, ShouldNotBeNil)
			So(bv2.Activated, ShouldBeFalse)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})
}

func findStatus(v *version.Version, buildVariant string) (*version.BuildStatus, bool) {
	for _, status := range v.BuildVariants {
		if status.BuildVariant == buildVariant {
			return &status, true
		}
	}
	return nil, false
}

func newTestRepoPollRevision(project string,
	activationTime time.Time) *model.Repository {
	return &model.Repository{
		Project:             project,
		RevisionOrderNumber: 0,
		LastRevision:        firstRevision,
	}
}

func createTestRevision(revision string,
	createTime time.Time) *model.Revision {
	return &model.Revision{
		Author:          "author",
		AuthorEmail:     "authorEmail",
		RevisionMessage: "revisionMessage",
		Revision:        revision,
		CreateTime:      createTime,
	}
}

func createTestProject(override1, override2 *int) *model.Project {
	return &model.Project{
		BuildVariants: []model.BuildVariant{
			model.BuildVariant{
				Name:        "bv1",
				DisplayName: "bv1",
				BatchTime:   override1,
				Tasks: []model.BuildVariantTask{
					model.BuildVariantTask{
						Name:    "Unabhaengigkeitserklaerungen",
						Distros: []string{"test-distro-one"},
					},
				},
			},
			model.BuildVariant{
				Name:        "bv2",
				DisplayName: "bv2",
				BatchTime:   override2,
				Tasks: []model.BuildVariantTask{
					model.BuildVariantTask{
						Name:    "Unabhaengigkeitserklaerungen",
						Distros: []string{"test-distro-one"},
					},
				},
			},
		},
		Tasks: []model.ProjectTask{
			model.ProjectTask{
				Name:     "Unabhaengigkeitserklaerungen",
				Commands: []model.PluginCommandConf{},
			},
		},
	}
}
