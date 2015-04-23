package repotracker

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/testutils"
	"10gen.com/mci/util"
	"errors"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
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
		mci.SetLogger(testConfig.RepoTracker.LogFile)
	}
}

func TestFetchRevisions(t *testing.T) {

	testutils.ConfigureIntegrationTest(t, testConfig, "TestFetchRevisions")

	Convey("With a GithubRepositoryPoller with a valid OAuth token...", t,
		func() {
			repoTracker := NewRepositoryTracker(
				testConfig, projectRef, NewGithubRepositoryPoller(projectRef,
					testConfig.Credentials["github"]), util.SystemClock{},
			)

			Convey("Fetching commits from the repository should not return "+
				"any errors", func() {
				So(repoTracker.FetchRevisions(10), ShouldBeNil)
			})

			Convey("Only get 3 revisions from the given repository if given a "+
				"limit of 4 commits where only 3 exist", func() {
				util.HandleTestingErr(repoTracker.FetchRevisions(4), t,
					"Error running repository process %v")
				numVersions, err := model.TotalVersions(bson.M{})
				util.HandleTestingErr(err, t, "Error finding all versions")
				So(numVersions, ShouldEqual, 3)
			})

			Convey("Only get 2 revisions from the given repository if given a "+
				"limit of 2 commits where 3 exist", func() {
				util.HandleTestingErr(repoTracker.FetchRevisions(2), t,
					"Error running repository process %v")
				numVersions, err := model.TotalVersions(bson.M{})
				util.HandleTestingErr(err, t, "Error finding all versions")
				So(numVersions, ShouldEqual, 2)
			})

			Reset(func() {
				dropTestDB(t)
			})
		})
}

func TestStoreRepositoryRevisions(t *testing.T) {
	testutils.ConfigureIntegrationTest(t, testConfig, "TestStoreRepositoryRevisions")
	Convey("When storing revisions gotten from a repository...", t, func() {
		repoTracker := NewRepositoryTracker(
			testConfig, projectRef, NewGithubRepositoryPoller(projectRef,
				testConfig.Credentials["github"]), util.SystemClock{},
		)

		Convey("On storing a single repo revision, we expect a version to be created"+
			" in the database for this project, which should be retrieved when we search"+
			" for this project's most recent version", func() {
			createTime := time.Now()
			revisionOne := *createTestRevision("firstRevision", createTime)
			revisions := []model.Revision{revisionOne}

			resultVersion, err := repoTracker.StoreRepositoryRevisions(revisions)
			util.HandleTestingErr(err, t, "Error storing repository revisions %v")

			newestVersion, err := model.FindMostRecentVersion(projectRef.String())
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

			_, err := repoTracker.StoreRepositoryRevisions(revisions)
			util.HandleTestingErr(err, t, "Error storing repository revisions %v")

			versionOne, err :=
				model.FindVersionByIdAndRevision(projectRef.String(), revisionOne.Revision)
			util.HandleTestingErr(err, t, "Error retrieving first stored version %v")
			versionTwo, err :=
				model.FindVersionByIdAndRevision(projectRef.String(), revisionTwo.Revision)
			util.HandleTestingErr(err, t, "Error retreiving second stored version %v")

			So(versionOne.Revision, ShouldEqual, revisionOne.Revision)
			So(versionTwo.Revision, ShouldEqual, revisionTwo.Revision)
		})
	})

	Convey("When storing versions from repositories with remote configuration files...", t,
		func() {

			project := createTestProject(10, nil, nil)

			revisions := []model.Revision{
				*createTestRevision("foo", time.Now().Add(1*time.Minute)),
			}

			poller := NewMockRepoPoller(project, revisions)

			repoTracker := NewRepositoryTracker(
				&mci.MCISettings{},
				&model.ProjectRef{
					Identifier: "testproject",
					Remote:     true,
				},
				poller,
				util.SystemClock{},
			)

			Convey("We should not fetch configs for versions we already have stored.",
				func() {
					So(poller.ConfigGets, ShouldBeZeroValue)
					// Store revisions the first time
					_, err := repoTracker.StoreRepositoryRevisions(revisions)
					So(err, ShouldBeNil)
					// We should have fetched the config once for each revision
					So(poller.ConfigGets, ShouldEqual, len(revisions))

					// Store them again
					_, err = repoTracker.StoreRepositoryRevisions(revisions)
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
					stubVersion, err := repoTracker.StoreRepositoryRevisions(revisions)
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
					v, err := repoTracker.StoreRepositoryRevisions(revisions)
					So(v, ShouldBeNil)
					So(err, ShouldEqual, unexpectedError)
				},
			)
		})
}

func TestBatchTimes(t *testing.T) {
	Convey("When deciding whether or not to activate variants for the most recently stored version",
		t, func() {
			now := time.Now()
			// We create a version with an activation time of now so that all the bvs have a last activation time
			previouslyActivatedVersion := model.Version{
				Project: "testproject",
				BuildVariants: []model.BuildStatus{
					model.BuildStatus{
						BuildVariant: "bv1",
						Activated:    true,
						ActivateAt:   now,
					},
					model.BuildStatus{
						BuildVariant: "bv2",
						Activated:    true,
						ActivateAt:   now,
					},
				},
				RevisionOrderNumber: 0,
				Requester:           mci.RepotrackerVersionRequester,
			}

			previouslyActivatedVersion.Insert()

			Convey("If the project's batch time has not elapsed, and no buildvariants "+
				"have overriden their batch times, no variants should be activated", func() {
				project := createTestProject(10, nil, nil)
				revisions := []model.Revision{
					*createTestRevision("foo", time.Now()),
				}

				repoTracker := NewRepositoryTracker(
					&mci.MCISettings{},
					&model.ProjectRef{
						Identifier: "testproject",
						Remote:     true,
					},
					NewMockRepoPoller(project, revisions),
					util.SystemClock{},
				)
				version, err := repoTracker.storeRevision(&revisions[0], project)
				So(err, ShouldBeNil)
				err = repoTracker.activateVersionVariantsIfNeeded(version)
				So(err, ShouldBeNil)
				So(len(version.BuildVariants), ShouldEqual, 2)
				So(version.BuildVariants[0].Activated, ShouldBeFalse)
				So(version.BuildVariants[1].Activated, ShouldBeFalse)
			})

			Convey("If the project's batch time has elasped, and no buildvariants "+
				"have overridden their batch times, all variants should be activated", func() {
				project := createTestProject(10, nil, nil)
				revisions := []model.Revision{
					*createTestRevision("bar", time.Now()),
				}
				repoTracker := NewRepositoryTracker(
					&mci.MCISettings{},
					&model.ProjectRef{
						Identifier: "testproject",
						Remote:     true,
					},
					NewMockRepoPoller(project, revisions),
					mockClock{time.Now().Add(time.Duration(4 * time.Hour))},
				)
				version, err := repoTracker.storeRevision(&revisions[0], project)
				So(err, ShouldBeNil)
				repoTracker.activateVersionVariantsIfNeeded(version)
				So(err, ShouldBeNil)
				bv1, found := findStatus(version, "bv1")
				So(found, ShouldBeTrue)
				So(bv1.Activated, ShouldBeTrue)
				bv2, found := findStatus(version, "bv2")
				So(found, ShouldBeTrue)
				So(bv2.Activated, ShouldBeTrue)
			})

			Convey("If the project's batch time has elasped, but both variants "+
				"have overridden their batch times (which have not elapsed)"+
				" , no variants should be activated", func() {
				// need to assign pointer vals
				twoforty := 240
				onetwenty := 120

				project := createTestProject(60, &twoforty, &onetwenty)

				revisions := []model.Revision{
					*createTestRevision("baz", time.Now()),
				}

				repoTracker := NewRepositoryTracker(
					&mci.MCISettings{},
					&model.ProjectRef{
						Identifier: "testproject",
						Remote:     true,
					},
					NewMockRepoPoller(project, revisions),
					mockClock{time.Now().Add(time.Duration(1) * time.Hour)}, // set time to 60 mins from now
				)
				version, err := repoTracker.storeRevision(&revisions[0], project)
				So(err, ShouldBeNil)
				err = repoTracker.activateVersionVariantsIfNeeded(version)
				So(err, ShouldBeNil)
				bv1, found := findStatus(version, "bv1")
				So(found, ShouldBeTrue)
				So(bv1.Activated, ShouldBeFalse)
				bv2, found := findStatus(version, "bv2")
				So(found, ShouldBeTrue)
				So(bv2.Activated, ShouldBeFalse)
			})

			Convey("If the project's batch time not elasped, but one variant "+
				"has overridden their batch times to be shorter"+
				", that variant should be activated", func() {
				ten := 10

				project := createTestProject(60, &ten, nil)

				revisions := []model.Revision{
					*createTestRevision("garply", time.Now()),
				}

				repoTracker := NewRepositoryTracker(
					&mci.MCISettings{},
					&model.ProjectRef{
						Identifier: "testproject",
						Remote:     true,
					},
					NewMockRepoPoller(project, revisions),
					mockClock{time.Now().Add(time.Duration(20) * time.Minute)}, // set time to 20 mins from now
				)
				version, err := repoTracker.storeRevision(&revisions[0], project)
				So(err, ShouldBeNil)
				err = repoTracker.activateVersionVariantsIfNeeded(version)
				So(err, ShouldBeNil)
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

func findStatus(version *model.Version, buildVariant string) (*model.BuildStatus, bool) {
	for _, status := range version.BuildVariants {
		if status.BuildVariant == buildVariant {
			return &status, true
		}
	}
	return nil, false
}

func newTestRepoPollRevision(project string,
	activationTime time.Time) *model.Repository {
	return &model.Repository{
		RepositoryProject:   project,
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

func createTestProject(projectBatchTime int, override1, override2 *int) *model.Project {
	return &model.Project{
		DisplayName: "Fake project",
		BatchTime:   projectBatchTime,
		BuildVariants: []model.BuildVariant{
			model.BuildVariant{
				Name:        "bv1",
				DisplayName: "bv1",
				BatchTime:   override1,
				Tasks: []model.BuildVariantTask{
					model.BuildVariantTask{
						Name:    "Unabhaengigkeitserklaerungen",
						Distros: []string{"ubuntu"},
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
						Distros: []string{"ubuntu"},
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
