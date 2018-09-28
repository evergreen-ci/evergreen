package repotracker

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestFetchRevisions(t *testing.T) {
	dropTestDB(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFetchRevisions")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Convey("With a GithubRepositoryPoller with a valid OAuth token...", t, func() {
		err := modelutil.CreateTestLocalConfig(testConfig, "mci-test", "")
		So(err, ShouldBeNil)
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)

		resetProjectRefs()

		repoTracker := RepoTracker{
			testConfig,
			evgProjectRef,
			NewGithubRepositoryPoller(evgProjectRef, token),
		}

		Convey("Fetching commits from the repository should not return any errors", func() {
			testConfig.RepoTracker.NumNewRepoRevisionsToFetch = 10
			So(repoTracker.FetchRevisions(ctx), ShouldBeNil)
		})

		Convey("Only get 2 revisions from the given repository if given a "+
			"limit of 2 commits where 3 exist", func() {
			testConfig.RepoTracker.NumNewRepoRevisionsToFetch = 2
			testutil.HandleTestingErr(repoTracker.FetchRevisions(ctx), t,
				"Error running repository process %s", repoTracker.Id)
			numVersions, err := version.Count(version.All)
			testutil.HandleTestingErr(err, t, "Error finding all versions")
			So(numVersions, ShouldEqual, 2)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})
}

func TestStoreRepositoryRevisions(t *testing.T) {
	dropTestDB(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestStoreRepositoryRevisions")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Convey("When storing revisions gotten from a repository...", t, func() {
		err := modelutil.CreateTestLocalConfig(testConfig, "mci-test", "")
		So(err, ShouldBeNil)
		token, err := testConfig.GetGithubOauthToken()
		So(err, ShouldBeNil)
		repoTracker := RepoTracker{testConfig, evgProjectRef, NewGithubRepositoryPoller(evgProjectRef, token)}

		// insert distros used in testing.
		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(), ShouldBeNil)

		Convey("On storing a single repo revision, we expect a version to be created"+
			" in the database for this project, which should be retrieved when we search"+
			" for this project's most recent version", func() {
			createTime := time.Now()
			revisionOne := *createTestRevision("1d97b5e8127a684f341d9fea5b3a2848f075c3b0", createTime)
			revisions := []model.Revision{revisionOne}

			resultVersion, err := repoTracker.StoreRevisions(ctx, revisions)
			testutil.HandleTestingErr(err, t, "Error storing repository revisions %s", revisionOne.Revision)

			newestVersion, err := version.FindOne(version.ByMostRecentSystemRequester(evgProjectRef.String()))
			testutil.HandleTestingErr(err, t, "Error retreiving newest version %s", newestVersion.Id)

			So(resultVersion, ShouldResemble, newestVersion)
			So(resultVersion.AuthorID, ShouldEqual, "")
		})

		Convey("On storing several repo revisions, we expect a version to be created "+
			"for each revision", func() {
			createTime := time.Now()
			laterCreateTime := createTime.Add(time.Duration(4 * time.Hour))

			revisionOne := *createTestRevision("1d97b5e8127a684f341d9fea5b3a2848f075c3b0", laterCreateTime)
			revisionTwo := *createTestRevision("d8e95fcffa1055fb9e2793fa47fec39d61dd1500", createTime)

			revisions := []model.Revision{revisionOne, revisionTwo}

			_, err := repoTracker.StoreRevisions(ctx, revisions)
			testutil.HandleTestingErr(err, t, "Error storing repository revisions %s, %s", revisionOne.Revision, revisionTwo.Revision)

			versionOne, err := version.FindOne(version.ByProjectIdAndRevision(evgProjectRef.Identifier, revisionOne.Revision))
			testutil.HandleTestingErr(err, t, "Error retrieving first stored version %s", versionOne.Id)
			versionTwo, err := version.FindOne(version.ByProjectIdAndRevision(evgProjectRef.Identifier, revisionTwo.Revision))
			testutil.HandleTestingErr(err, t, "Error retreiving second stored version %s", versionTwo.Revision)

			So(versionOne.Revision, ShouldEqual, revisionOne.Revision)
			So(versionTwo.Revision, ShouldEqual, revisionTwo.Revision)
			So(versionOne.AuthorID, ShouldEqual, "")
			So(versionTwo.AuthorID, ShouldEqual, "")
		})
		Convey("if an evergreen user can be associated with the commit, record it", func() {
			revisionOne := *createTestRevision("1d97b5e8127a684f341d9fea5b3a2848f075c3b0", time.Now())
			revisions := []model.Revision{revisionOne}
			revisions[0].AuthorGithubUID = 1234

			u := user.DBUser{
				Id: "testUser",
				Settings: user.UserSettings{
					GithubUser: user.GithubUser{
						UID:         1234,
						LastKnownAs: "somebody",
					},
				},
			}
			So(u.Insert(), ShouldBeNil)

			_, err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			versionOne, err := version.FindOne(version.ByProjectIdAndRevision(evgProjectRef.Identifier, revisionOne.Revision))
			So(err, ShouldBeNil)
			So(versionOne.AuthorID, ShouldEqual, "testUser")

			u2, err := user.FindOne(user.ById("testUser"))
			So(err, ShouldBeNil)
			So(u2, ShouldNotBeNil)
			So(u2.Settings.GithubUser.LastKnownAs, ShouldEqual, "somebody")
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
				_, err := repoTracker.StoreRevisions(ctx, revisions)
				So(err, ShouldBeNil)
				// We should have fetched the config once for each revision
				So(poller.ConfigGets, ShouldEqual, len(revisions))

				// Store them again
				_, err = repoTracker.StoreRevisions(ctx, revisions)
				So(err, ShouldBeNil)
				// We shouldn't have fetched the config any additional times
				// since we have already stored these versions
				So(poller.ConfigGets, ShouldEqual, len(revisions))
			},
		)

		Convey("We should handle invalid configuration files gracefully by storing a stub version", func() {
			errStrs := []string{"Someone dun' goof'd"}
			poller.setNextError(projectConfigError{errStrs, []string{}})
			stubVersion, err := repoTracker.StoreRevisions(ctx, revisions)
			// We want this error to get swallowed so a config error
			// doesn't stop additional versions from getting created
			So(err, ShouldBeNil)
			So(stubVersion.Errors, ShouldResemble, errStrs)
			So(len(stubVersion.BuildVariants), ShouldEqual, 0)
		})

		Convey("Project configuration files with missing distros should still create versions", func() {
			poller.addBadDistro("Cray-Y-MP")
			v, err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			So(v, ShouldNotBeNil)
			So(len(v.BuildVariants), ShouldBeGreaterThan, 0)

			Convey("and log a warning", func() {
				So(len(v.Warnings), ShouldEqual, 1)
				So(v.Errors, ShouldBeNil)
			})
		})

		Convey("If there is an error other than a config error while fetching a config, we should fail hard",
			func() {
				unexpectedError := errors.New("Something terrible has happened!!")
				poller.setNextError(unexpectedError)
				v, err := repoTracker.StoreRevisions(ctx, revisions)
				So(v, ShouldBeNil)
				So(err.Error(), ShouldEqual, unexpectedError.Error())
			},
		)

		Reset(func() {
			dropTestDB(t)
		})

	})
}

func TestBatchTimes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When deciding whether or not to activate variants for the most recently stored version", t, func() {
		// We create a version with an activation time of now so that all the bvs have a last activation time of now.
		So(db.ClearCollections(version.Collection, distro.Collection), ShouldBeNil)
		previouslyActivatedVersion := version.Version{
			Id:         "previously activated",
			Identifier: "testproject",
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
			"have overridden their batch times, no variants should be activated", func() {
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
			v, err := repoTracker.StoreRevisions(ctx, revisions)
			So(v, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(len(v.BuildVariants), ShouldEqual, 2)
			So(model.ActivateElapsedBuilds(v), ShouldBeNil)
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
			version, err := repoTracker.StoreRevisions(ctx, revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(model.ActivateElapsedBuilds(version), ShouldBeNil)
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
			version, err := repoTracker.StoreRevisions(ctx, revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(model.ActivateElapsedBuilds(version), ShouldBeNil)
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
			version, err := repoTracker.StoreRevisions(ctx, revisions)
			So(version, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(model.ActivateElapsedBuilds(version), ShouldBeNil)
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

	Convey("If the new revision adds a variant", t, func() {
		previouslyActivatedVersion := version.Version{
			Id:         "previously activated",
			Identifier: "testproject",
			BuildVariants: []version.BuildStatus{
				{
					BuildVariant: "bv1",
					Activated:    true,
					ActivateAt:   time.Now(),
				},
				// "bv2" will be added in a later revision
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
		version, err := repoTracker.StoreRevisions(ctx, revisions)
		So(version, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("the new variant should activate immediately", func() {
			So(model.ActivateElapsedBuilds(version), ShouldBeNil)
			bv1, found := findStatus(version, "bv1")
			So(found, ShouldBeTrue)
			So(bv1.Activated, ShouldBeTrue)
			bv2, found := findStatus(version, "bv2")
			So(found, ShouldBeTrue)
			So(bv2, ShouldNotBeNil)
			So(bv2.Activated, ShouldBeTrue)
			So(bv2.ActivateAt, ShouldResemble, bv1.ActivateAt)
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
			{
				Name:        "bv1",
				DisplayName: "bv1",
				BatchTime:   override1,
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "Unabhaengigkeitserklaerungen",
						Distros: []string{"test-distro-one"},
					},
				},
			},
			{
				Name:        "bv2",
				DisplayName: "bv2",
				BatchTime:   override2,
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "Unabhaengigkeitserklaerungen",
						Distros: []string{"test-distro-one"},
					},
				},
			},
		},
		Tasks: []model.ProjectTask{
			{
				Name:     "Unabhaengigkeitserklaerungen",
				Commands: []model.PluginCommandConf{},
			},
		},
	}
}

func TestBuildBreakSubscriptions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(user.Collection))
	u := user.DBUser{
		Id:           "me",
		EmailAddress: "toki@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceEmail,
			},
		},
	}
	assert.NoError(u.Insert())

	// no notifications in project or user
	subs := []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	proj1 := model.ProjectRef{
		Identifier:           "proj1",
		NotifyOnBuildFailure: false,
	}
	v1 := version.Version{
		Id:         "v1",
		Identifier: proj1.Identifier,
		Requester:  evergreen.RepotrackerVersionRequester,
		Branch:     "branch",
	}
	assert.NoError(addBuildBreakSubscriptions(&v1, &proj1))
	assert.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Q{}, &subs))
	assert.Len(subs, 0)

	// just a project
	subs = []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	proj2 := model.ProjectRef{
		Identifier:           "proj2",
		NotifyOnBuildFailure: true,
		Admins:               []string{"u2", "u3"},
	}
	u2 := user.DBUser{
		Id:           "u2",
		EmailAddress: "shaw@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceEmail,
			},
		},
	}
	assert.NoError(u2.Insert())
	u3 := user.DBUser{
		Id:           "u3",
		EmailAddress: "tess@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceEmail,
			},
		},
	}
	assert.NoError(u3.Insert())
	assert.NoError(addBuildBreakSubscriptions(&v1, &proj2))
	assert.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Q{}, &subs))
	assert.Len(subs, 2)

	// project has it enabled, but user doesn't want notifications
	subs = []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	u4 := user.DBUser{
		Id:           "u4",
		EmailAddress: "rehgar@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{},
		},
	}
	assert.NoError(u4.Insert())
	v3 := version.Version{
		Id:         "v3",
		Identifier: proj1.Identifier,
		Requester:  evergreen.RepotrackerVersionRequester,
		Branch:     "branch",
		AuthorID:   u4.Id,
	}
	assert.NoError(addBuildBreakSubscriptions(&v3, &proj2))
	assert.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Q{}, &subs))
	assert.Len(subs, 2)
}
