package repotracker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() { testutil.Setup() }

func TestFetchRevisions(t *testing.T) {
	dropTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Convey("With a GithubRepositoryPoller", t, func() {
		err := modelutil.CreateTestLocalConfig(t.Context(), testConfig, "mci-test", "")
		So(err, ShouldBeNil)

		resetProjectRefs()
		repoTracker := RepoTracker{
			testConfig,
			evgProjectRef,
			NewGithubRepositoryPoller(evgProjectRef),
		}

		Convey("Fetching commits from the repository should not return any errors", func() {
			testConfig.RepoTracker.NumNewRepoRevisionsToFetch = 10
			So(repoTracker.FetchRevisions(ctx), ShouldBeNil)
		})

		Convey("Fetching commits for a disabled repotracker should create no versions", func() {
			evgProjectRef.RepotrackerDisabled = utility.TruePtr()
			So(repoTracker.FetchRevisions(ctx), ShouldBeNil)
			numVersions, err := model.VersionCount(t.Context(), model.VersionAll)
			require.NoError(t, err, "Error finding all versions")
			So(numVersions, ShouldEqual, 0)
			evgProjectRef.RepotrackerDisabled = utility.FalsePtr()
		})

		Convey("Only get 2 revisions from the given repository if given a "+
			"limit of 2 commits where 3 exist", func() {
			testutil.ConfigureIntegrationTest(t, testConfig)
			testConfig.RepoTracker.NumNewRepoRevisionsToFetch = 2
			require.NoError(t, repoTracker.FetchRevisions(ctx),
				"Error running repository process %s", repoTracker.Settings.Id)
			numVersions, err := model.VersionCount(t.Context(), model.VersionAll)
			require.NoError(t, err, "Error finding all versions")
			So(numVersions, ShouldEqual, 2)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})
}

func TestStoreRepositoryRevisions(t *testing.T) {
	dropTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Convey("When storing revisions gotten from a repository...", t, func() {
		err := modelutil.CreateTestLocalConfig(t.Context(), testConfig, "mci-test", "")
		So(err, ShouldBeNil)
		repoTracker := RepoTracker{testConfig, evgProjectRef, NewGithubRepositoryPoller(evgProjectRef)}

		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(ctx), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(ctx), ShouldBeNil)

		Convey("On storing a single repo revision, we expect a version to be created"+
			" in the database for this project, which should be retrieved when we search"+
			" for this project's most recent version", func() {
			testutil.ConfigureIntegrationTest(t, testConfig)
			createTime := time.Now()
			revisionOne := *createTestRevision("1d97b5e8127a684f341d9fea5b3a2848f075c3b0", createTime)
			revisions := []model.Revision{revisionOne}

			err := repoTracker.StoreRevisions(ctx, revisions)
			require.NoError(t, err, "Error storing repository revisions %s", revisionOne.Revision)

			newestVersion, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester(evgProjectRef.Id))
			require.NoError(t, err, "Error retreiving newest version")
			require.NotNil(t, newestVersion)

			So(newestVersion.AuthorID, ShouldEqual, "")
		})

		Convey("On storing several repo revisions, we expect a version to be created "+
			"for each revision", func() {
			testutil.ConfigureIntegrationTest(t, testConfig)
			createTime := time.Now()
			laterCreateTime := createTime.Add(4 * time.Hour)

			revisionOne := *createTestRevision("1d97b5e8127a684f341d9fea5b3a2848f075c3b0", laterCreateTime)
			revisionTwo := *createTestRevision("d8e95fcffa1055fb9e2793fa47fec39d61dd1500", createTime)

			revisions := []model.Revision{revisionOne, revisionTwo}

			err := repoTracker.StoreRevisions(ctx, revisions)
			require.NoError(t, err, "Error storing repository revisions %s, %s", revisionOne.Revision, revisionTwo.Revision)

			versionOne, err := model.VersionFindOne(t.Context(), model.BaseVersionByProjectIdAndRevision(evgProjectRef.Id, revisionOne.Revision))
			require.NoError(t, err, "Error retrieving first stored version")
			require.NotNil(t, versionOne)
			versionTwo, err := model.VersionFindOne(t.Context(), model.BaseVersionByProjectIdAndRevision(evgProjectRef.Id, revisionTwo.Revision))
			require.NoError(t, err, "Error retreiving second stored version")
			require.NotNil(t, versionTwo)

			So(versionOne.Revision, ShouldEqual, revisionOne.Revision)
			So(versionTwo.Revision, ShouldEqual, revisionTwo.Revision)
			So(versionOne.AuthorID, ShouldEqual, "")
			So(versionTwo.AuthorID, ShouldEqual, "")
		})
		Convey("if an evergreen user can be associated with the commit, record it", func() {
			testutil.ConfigureIntegrationTest(t, testConfig)
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
			So(u.Insert(t.Context()), ShouldBeNil)

			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			versionOne, err := model.VersionFindOne(t.Context(), model.BaseVersionByProjectIdAndRevision(evgProjectRef.Id, revisionOne.Revision))
			So(err, ShouldBeNil)
			So(versionOne, ShouldNotBeNil)
			So(versionOne.AuthorID, ShouldEqual, "testUser")

			u2, err := user.FindOne(ctx, user.ById("testUser"))
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

		pRef := &model.ProjectRef{
			Id:        "testproject",
			BatchTime: 10,
		}
		require.NoError(t, pRef.Insert(t.Context()))

		repoTracker := RepoTracker{
			testConfig,
			pRef,
			poller,
		}

		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(ctx), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(ctx), ShouldBeNil)

		Convey("We should not fetch configs for versions we already have stored.",
			func() {
				So(poller.ConfigGets, ShouldBeZeroValue)
				// Store revisions the first time
				err := repoTracker.StoreRevisions(ctx, revisions)
				So(err, ShouldBeNil)
				// We should have fetched the config once for each revision
				So(poller.ConfigGets, ShouldEqual, len(revisions))

				// Store them again
				err = repoTracker.StoreRevisions(ctx, revisions)
				So(err, ShouldBeNil)
				// We shouldn't have fetched the config any additional times
				// since we have already stored these versions
				So(poller.ConfigGets, ShouldEqual, len(revisions))
			},
		)

		Convey("We should handle invalid configuration files gracefully by storing a stub version", func() {
			errStrs := []string{"Someone dun' goof'd"}
			poller.setNextError(projectConfigError{errStrs, []string{}})
			err := repoTracker.StoreRevisions(ctx, revisions)
			// We want this error to get swallowed so a config error
			// doesn't stop additional versions from getting created
			So(err, ShouldBeNil)
			stubVersion, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(stubVersion, ShouldNotBeNil)
			So(stubVersion.Errors, ShouldResemble, errStrs)
			So(len(stubVersion.BuildVariants), ShouldEqual, 0)
		})

		Convey("We should handle invalid configuration files with merging errors gracefully by storing a stub version", func() {
			poller.setNextError(errors.New(model.MergeProjectConfigError))
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			stubVersion, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(stubVersion, ShouldNotBeNil)
			So(stubVersion.Errors[0], ShouldContainSubstring, model.MergeProjectConfigError)
			So(len(stubVersion.BuildVariants), ShouldEqual, 0)
		})

		Convey("Project configuration files with missing distros should still create versions", func() {
			poller.addBadDistro("Cray-Y-MP")
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
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
				err := repoTracker.StoreRevisions(ctx, revisions)
				So(err.Error(), ShouldEqual, unexpectedError.Error())
				v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
				So(err, ShouldBeNil)
				So(v, ShouldBeNil)
			},
		)

		Convey("Should create build break notifications for project admins", func() {
			u := user.DBUser{
				Id:           "me",
				EmailAddress: "me@example.com",
				Settings: user.UserSettings{
					Notifications: user.NotificationPreferences{
						BuildBreak: user.PreferenceEmail,
					},
				},
			}
			So(u.Insert(t.Context()), ShouldBeNil)
			pRef.NotifyOnBuildFailure = utility.TruePtr()
			pRef.Admins = []string{u.Id}

			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(v, ShouldNotBeNil)
			So(len(v.BuildVariants), ShouldBeGreaterThan, 0)

			subs, err := event.FindSubscriptionsByAttributes(ctx, event.ResourceTypeTask, event.Attributes{
				Object:    []string{event.ObjectTask},
				Project:   []string{pRef.Id},
				Requester: []string{evergreen.RepotrackerVersionRequester},
				InVersion: []string{v.Id},
			})
			So(err, ShouldBeNil)
			So(len(subs), ShouldEqual, 1)
		})

		Convey("Should set the most recent created version as the latest revision even if there is error setting up build break notifications", func() {
			pRef.NotifyOnBuildFailure = utility.TruePtr()
			pRef.Admins = []string{"nonexistent_user"}

			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(v, ShouldNotBeNil)
			So(len(v.BuildVariants), ShouldBeGreaterThan, 0)

			subs, err := event.FindSubscriptionsByAttributes(ctx, event.ResourceTypeTask, event.Attributes{
				Object:    []string{event.ObjectTask},
				Project:   []string{pRef.Id},
				Requester: []string{evergreen.RepotrackerVersionRequester},
				InVersion: []string{v.Id},
			})
			So(err, ShouldBeNil)
			So(len(subs), ShouldEqual, 0)

			repo, err := model.FindRepository(ctx, pRef.Id)
			So(err, ShouldBeNil)
			So(repo, ShouldNotBeNil)
			So(repo.LastRevision, ShouldEqual, v.Revision)
		})

		Reset(func() {
			dropTestDB(t)
		})

	})
}

func TestCountNumDependentsAcrossVariants(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(model.VersionCollection, distro.Collection, model.ParserProjectCollection,
		build.Collection, task.Collection, model.ProjectConfigCollection, model.ProjectRefCollection))

	simpleYml := `
buildvariants:
- name: bv1
  display_name: "bv_display"
  run_on: d1
  tasks:
  - name: t1
  - name: t2
  - name: t3
  - name: t4
- name: bv2
  display_name: bv2_display
  run_on: d2
  tasks:
  - name: t1
- name: bv3
  display_name: bv3_display
  run_on: d2
  tasks:
  - name: t4
    depends_on:
    - name: t1
      variant: bv1
tasks:
- name: t1
- name: t2
  depends_on:
  - name: t1
- name: t3
  depends_on:
  - name: t1
- name: t4
  depends_on:
  - name: t2
  - name: t3
`
	previouslyActivatedVersion := &model.Version{
		Id:         "previously activated",
		Identifier: "testproject",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []model.VersionBuildStatus{
			{
				BuildVariant: "bv1",
				BatchTimeTasks: []model.BatchTimeTaskStatus{
					{
						TaskName: "t1",
						ActivationStatus: model.ActivationStatus{
							Activated:  true,
							ActivateAt: time.Now().Add(-11 * time.Minute),
						},
					},
				},
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-11 * time.Minute),
				},
			},
			{
				BuildVariant: "bv2",
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-11 * time.Minute),
				},
			},
		},
	}
	require.NoError(t, previouslyActivatedVersion.Insert(t.Context()))

	d := distro.Distro{Id: "d1"}
	require.NoError(t, d.Insert(ctx))
	d.Id = "d2"
	require.NoError(t, d.Insert(ctx))

	pRef := &model.ProjectRef{
		Id:        "testproject",
		BatchTime: 0,
	}
	require.NoError(t, pRef.Insert(t.Context()))

	p := &model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(simpleYml), nil, "testproject", p)
	assert.NoError(t, err)
	require.NotNil(t, pp)

	// Create new version to use for activating
	revisions := []model.Revision{
		*createTestRevision("yes", time.Now()),
	}
	repoTracker := RepoTracker{
		testConfig,
		pRef,
		NewMockRepoPoller(pp, revisions),
	}
	assert.NoError(t, repoTracker.StoreRevisions(ctx, revisions))

	bv1t1, err := task.FindOne(ctx, db.Query(bson.M{task.BuildVariantKey: "bv1", task.DisplayNameKey: "t1"}))
	require.NoError(t, err)
	require.NotNil(t, bv1t1)
	assert.Equal(t, 4, bv1t1.NumDependents)
}

func TestBatchTimeForTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(model.VersionCollection, distro.Collection, model.ParserProjectCollection,
		build.Collection, task.Collection, model.ProjectConfigCollection, model.ProjectRefCollection))

	simpleYml := `
buildvariants:
- name: bv1
  display_name: "bv_display"
  run_on: d1
  batchtime: 10
  tasks:
  - name: t1
    batchtime: 30
  - name: t2
  - name: t3
- name: bv2
  display_name: bv2_display
  run_on: d2
  tasks:
  - name: t1
- name: bv3
  display_name: bv3_display
  run_on: d2
  tasks:
  - name: t4
tasks:
- name: t1
  priority: 3
- name: t2
  priority: -1
- name: t3
  depends_on:
  - name: t4
    variant: bv3
- name: t4
  priority: -1
`
	previouslyActivatedVersion := &model.Version{
		Id:         "previously activated",
		Identifier: "testproject",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []model.VersionBuildStatus{
			{
				BuildVariant: "bv1",
				BatchTimeTasks: []model.BatchTimeTaskStatus{
					{
						TaskName: "t1",
						ActivationStatus: model.ActivationStatus{
							Activated:  true,
							ActivateAt: time.Now().Add(-11 * time.Minute),
						},
					},
				},
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-11 * time.Minute),
				},
			},
			{
				BuildVariant: "bv2",
				ActivationStatus: model.ActivationStatus{
					Activated:  true,
					ActivateAt: time.Now().Add(-11 * time.Minute),
				},
			},
		},
	}
	assert.NoError(t, previouslyActivatedVersion.Insert(t.Context()))

	d := distro.Distro{Id: "d1"}
	assert.NoError(t, d.Insert(ctx))
	d.Id = "d2"
	assert.NoError(t, d.Insert(ctx))

	pRef := &model.ProjectRef{
		Id:        "testproject",
		BatchTime: 0,
	}
	require.NoError(t, pRef.Insert(t.Context()))

	p := &model.Project{}
	pp, err := model.LoadProjectInto(ctx, []byte(simpleYml), nil, "testproject", p)
	assert.NoError(t, err)

	// Create new version to use for activating
	revisions := []model.Revision{
		*createTestRevision("yes", time.Now()),
	}
	repoTracker := RepoTracker{
		testConfig,
		pRef,
		NewMockRepoPoller(pp, revisions),
	}
	assert.NoError(t, repoTracker.StoreRevisions(ctx, revisions))
	v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Len(t, v.BuildVariants, 3)
	assert.False(t, v.BuildVariants[0].Activated)
	assert.False(t, v.BuildVariants[1].Activated)
	bv, _ := findStatus(v, "bv1")
	assert.Len(t, bv.BatchTimeTasks, 1)
	assert.False(t, bv.BatchTimeTasks[0].Activated)

	// should activate build variants and tasks except for the batchtime task and the task with a negative priority
	ok, err := model.ActivateElapsedBuildsAndTasks(ctx, v)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Len(t, v.BuildVariants, 3)
	assert.True(t, v.BuildVariants[0].Activated)
	assert.True(t, v.BuildVariants[1].Activated)
	assert.True(t, v.BuildVariants[2].Activated)
	bv, _ = findStatus(v, "bv1")
	assert.Len(t, bv.BatchTimeTasks, 1)
	assert.False(t, bv.BatchTimeTasks[0].Activated)

	build1, err := build.FindOneId(t.Context(), bv.BuildId)
	assert.NoError(t, err)
	require.NotZero(t, build1)

	// neither batchtime task nor disabled task should be activated
	tasks, err := task.Find(ctx, task.ByBuildId(build1.Id))
	assert.NoError(t, err)
	require.NotEmpty(t, tasks)
	assert.Len(t, tasks, 3)
	for _, tsk := range tasks {
		if tsk.DisplayName == "t1" {
			assert.False(t, tsk.Activated)
		}
		if tsk.DisplayName == "t2" {
			assert.False(t, tsk.Activated)
		}
		if tsk.DisplayName == "t3" {
			assert.True(t, tsk.Activated)
		}
	}

	bv, _ = findStatus(v, "bv3")

	build3, err := build.FindOneId(t.Context(), bv.BuildId)
	assert.NoError(t, err)
	require.NotZero(t, build3)

	tasks, err = task.Find(ctx, task.ByBuildId(build3.Id))
	assert.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, evergreen.DisabledTaskPriority, tasks[0].Priority)
	assert.False(t, tasks[0].Activated)

	// now we should update just the task even though the build is activated already
	for i, bv := range v.BuildVariants {
		if bv.BuildVariant == "bv1" {
			// Set the activation time before the current timestamp to ensure
			// that it is already elapsed.
			v.BuildVariants[i].BatchTimeTasks[0].ActivateAt = time.Now().Add(-time.Millisecond)
		}
	}
	ok, err = model.ActivateElapsedBuildsAndTasks(ctx, v)
	assert.NoError(t, err)
	assert.True(t, ok)
	bv, _ = findStatus(v, "bv1")
	assert.Len(t, bv.BatchTimeTasks, 1)
	assert.True(t, bv.BatchTimeTasks[0].Activated)

	// validate that the activation time of the entire build was not changed
	build2, err := build.FindOneId(t.Context(), bv.BuildId)
	assert.NoError(t, err)
	assert.Equal(t, build1.ActivatedTime, build2.ActivatedTime)
}

func TestBatchTimes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When deciding whether or not to activate variants for the most recently stored version", t, func() {
		// We create a version with an activation time of now so that all the bvs have a last activation time of now.
		So(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, distro.Collection, model.ParserProjectCollection), ShouldBeNil)
		previouslyActivatedVersion := model.Version{
			Id:         "previously activated",
			Identifier: "testproject",
			BuildVariants: []model.VersionBuildStatus{
				{
					BuildVariant: "bv1",
					ActivationStatus: model.ActivationStatus{
						Activated:  true,
						ActivateAt: time.Now(),
					},
				},
				{
					BuildVariant: "bv2",
					ActivationStatus: model.ActivationStatus{
						Activated:  true,
						ActivateAt: time.Now(),
					},
				},
			},
			RevisionOrderNumber: 0,
			Requester:           evergreen.RepotrackerVersionRequester,
		}

		So(previouslyActivatedVersion.Insert(t.Context()), ShouldBeNil)

		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(ctx), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(ctx), ShouldBeNil)

		pRef := &model.ProjectRef{
			Id:        "testproject",
			BatchTime: 1,
		}
		So(pRef.Insert(t.Context()), ShouldBeNil)

		Convey("If the project's batch time has not elapsed, and no buildvariants "+
			"have overridden their batch times, no variants should be activated", func() {
			project := createTestProject(nil, nil)
			revisions := []model.Revision{
				*createTestRevision("foo", time.Now()),
			}

			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Id:        "testproject",
					BatchTime: 1,
				},
				NewMockRepoPoller(project, revisions),
			}
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			v, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(v, ShouldNotBeNil)
			So(len(v.BuildVariants), ShouldEqual, 2)
			ok, err := model.ActivateElapsedBuildsAndTasks(ctx, v)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
			So(v.BuildVariants[0].Activated, ShouldBeFalse)
			So(v.BuildVariants[1].Activated, ShouldBeFalse)
		})

		Convey("If the project's batch time has elapsed, and no buildvariants "+
			"have overridden their batch times, all variants should be activated", func() {
			project := createTestProject(nil, nil)
			revisions := []model.Revision{
				*createTestRevision("bar", time.Now().Add(-6*time.Minute)),
			}
			repoTracker := RepoTracker{
				testConfig,
				&model.ProjectRef{
					Id:        "testproject",
					BatchTime: 0,
				},
				NewMockRepoPoller(project, revisions),
			}
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			version, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(version, ShouldNotBeNil)
			ok, err := model.ActivateElapsedBuildsAndTasks(ctx, version)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
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
					Id:        "testproject",
					BatchTime: 60,
				},
				NewMockRepoPoller(project, revisions),
			}
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			version, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(version, ShouldNotBeNil)
			ok, err := model.ActivateElapsedBuildsAndTasks(ctx, version)
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
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
					Id:        "testproject",
					BatchTime: 60,
				},
				NewMockRepoPoller(project, revisions),
			}
			err := repoTracker.StoreRevisions(ctx, revisions)
			So(err, ShouldBeNil)
			version, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
			So(err, ShouldBeNil)
			So(version, ShouldNotBeNil)
			ok, err := model.ActivateElapsedBuildsAndTasks(ctx, version)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
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
		previouslyActivatedVersion := model.Version{
			Id:         "previously activated",
			Identifier: "testproject",
			BuildVariants: []model.VersionBuildStatus{
				{
					BuildVariant: "bv1",
					ActivationStatus: model.ActivationStatus{
						Activated:  true,
						ActivateAt: time.Now(),
					},
				},
				// "bv2" will be added in a later revision
			},
			RevisionOrderNumber: 0,
			Requester:           evergreen.RepotrackerVersionRequester,
		}
		So(previouslyActivatedVersion.Insert(t.Context()), ShouldBeNil)
		d := distro.Distro{Id: "test-distro-one"}
		So(d.Insert(ctx), ShouldBeNil)
		d.Id = "test-distro-two"
		So(d.Insert(ctx), ShouldBeNil)
		pRef := &model.ProjectRef{
			Id:        "testproject",
			BatchTime: 1,
		}
		So(pRef.Insert(t.Context()), ShouldBeNil)
		zero := 0
		project := createTestProject(&zero, nil)
		revisions := []model.Revision{
			*createTestRevision("garply", time.Now()),
		}
		repoTracker := RepoTracker{
			testConfig,
			&model.ProjectRef{
				Id:        "testproject",
				BatchTime: 60,
			},
			NewMockRepoPoller(project, revisions),
		}
		err := repoTracker.StoreRevisions(ctx, revisions)
		So(err, ShouldBeNil)
		version, err := model.VersionFindOne(t.Context(), model.VersionByMostRecentSystemRequester("testproject"))
		So(err, ShouldBeNil)
		So(version, ShouldNotBeNil)

		Convey("the new variant should activate immediately", func() {
			ok, err := model.ActivateElapsedBuildsAndTasks(ctx, version)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
			bv1, found := findStatus(version, "bv1")
			So(found, ShouldBeTrue)
			So(bv1.Activated, ShouldBeTrue)
			bv2, found := findStatus(version, "bv2")
			So(found, ShouldBeTrue)
			So(bv2, ShouldNotBeNil)
			So(bv2.Activated, ShouldBeTrue)
			So(bv2.DisplayName, ShouldEqual, "bv2")
			So(bv2.ActivateAt, ShouldResemble, bv1.ActivateAt)
		})

		Reset(func() {
			dropTestDB(t)
		})
	})
}

func findStatus(v *model.Version, buildVariant string) (*model.VersionBuildStatus, bool) {
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

func createTestProject(override1, override2 *int) *model.ParserProject {
	pp := &model.ParserProject{}
	pp.AddBuildVariant("bv1", "bv1", "", override1, []string{"t1"})
	pp.BuildVariants[0].Tasks[0].RunOn = []string{"test-distro-one"}

	pp.AddBuildVariant("bv2", "bv2", "", override2, []string{"t1"})
	pp.BuildVariants[1].Tasks[0].RunOn = []string{"test-distro-one"}

	pp.AddTask("t1", []model.PluginCommandConf{{
		Command: "shell.exec",
		Params: map[string]any{
			"script": "echo hi",
		},
	}})
	pp.Tasks[0].ExecTimeoutSecs = 3

	return pp
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
	assert.NoError(u.Insert(t.Context()))

	// no notifications in project or user
	subs := []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	proj1 := model.ProjectRef{
		Id:                   "proj1",
		NotifyOnBuildFailure: utility.FalsePtr(),
	}
	v1 := model.Version{
		Id:         "v1",
		Identifier: proj1.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		Branch:     "branch",
	}
	assert.NoError(AddBuildBreakSubscriptions(t.Context(), &v1, &proj1))
	assert.NoError(db.FindAllQ(t.Context(), event.SubscriptionsCollection, db.Q{}, &subs))
	assert.Empty(subs)

	// just a project
	subs = []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	proj2 := model.ProjectRef{
		Id:                   "proj2",
		NotifyOnBuildFailure: utility.TruePtr(),
		Admins:               []string{"u2", "u3", "u4"},
	}
	u2 := user.DBUser{
		Id:           "u2",
		EmailAddress: "shaw@blizzard.com",
		Settings: user.UserSettings{
			SlackUsername: "hello.itsme",
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceSlack,
			},
		},
	}
	assert.NoError(u2.Insert(t.Context()))
	u3 := user.DBUser{
		Id:           "u3",
		EmailAddress: "tess@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				BuildBreak: user.PreferenceEmail,
			},
		},
	}
	assert.NoError(u3.Insert(t.Context()))
	u4 := user.DBUser{
		Id:           "u4",
		EmailAddress: "rehgar@blizzard.com",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{},
		},
	}
	assert.NoError(u4.Insert(t.Context()))
	assert.NoError(AddBuildBreakSubscriptions(t.Context(), &v1, &proj2))
	assert.NoError(db.FindAllQ(t.Context(), event.SubscriptionsCollection, db.Q{}, &subs))
	assert.Len(subs, 2)

	// project has it enabled, but user doesn't want notifications
	subs = []event.Subscription{}
	assert.NoError(db.Clear(event.SubscriptionsCollection))
	v3 := model.Version{
		Id:         "v3",
		Identifier: proj2.Id,
		Requester:  evergreen.RepotrackerVersionRequester,
		Branch:     "branch",
		AuthorID:   u4.Id,
	}
	assert.NoError(AddBuildBreakSubscriptions(t.Context(), &v3, &proj2))
	assert.NoError(db.FindAllQ(t.Context(), event.SubscriptionsCollection, db.Q{}, &subs))
	targetString, ok := subs[0].Subscriber.Target.(*string)
	assert.True(ok)
	assert.EqualValues("@hello.itsme", utility.FromStringPtr(targetString))
	assert.Len(subs, 2)
}

type CreateVersionFromConfigSuite struct {
	ref           *model.ProjectRef
	rev           *model.Revision
	d             *distro.Distro
	sourceVersion *model.Version
	ctx           context.Context
	cancel        context.CancelFunc
	env           evergreen.Environment
	suite.Suite
}

func TestCreateVersionFromConfigSuite(t *testing.T) {
	suite.Run(t, new(CreateVersionFromConfigSuite))
}

func (s *CreateVersionFromConfigSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, model.ParserProjectCollection, build.Collection, task.Collection, distro.Collection, model.ProjectAliasCollection))
	s.ref = &model.ProjectRef{
		Repo:       "evergreen",
		Owner:      "evergreen-ci",
		Id:         "mci",
		Branch:     "main",
		RemotePath: "self-tests.yml",
		Enabled:    true,
	}
	s.NoError(s.ref.Insert(s.ctx))
	s.rev = &model.Revision{
		Author:          "me",
		AuthorGithubUID: 123,
		Revision:        "abc",
		RevisionMessage: "message",
		CreateTime:      time.Now(),
	}
	s.d = &distro.Distro{
		Id: "d",
	}
	s.sourceVersion = &model.Version{
		Id:       "v",
		Revision: "abc",
	}
	s.NoError(s.d.Insert(s.ctx))
	env := &mock.Environment{}
	s.Require().NoError(env.Configure(s.ctx))
	s.env = env
}

func (s *CreateVersionFromConfigSuite) TearDownTest() {
	s.cancel()
}

func (s *CreateVersionFromConfigSuite) TestCreateBasicVersion() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
- name: task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev, SourceVersion: s.sourceVersion}, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)

	dbVersion, err := model.VersionFindOneId(s.ctx, v.Id)
	s.NoError(err)
	s.NotNil(dbVersion)
	s.Equal(evergreen.VersionCreated, dbVersion.Status)
	s.Equal(s.rev.RevisionMessage, dbVersion.Message)
	s.Equal(evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod, "storage method should initially be DB for new versions")

	s.Equal(evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod)
	dbParserProject, err := model.ParserProjectFindOneByID(s.ctx, s.env.Settings(), dbVersion.ProjectStorageMethod, dbVersion.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbParserProject)
	s.Len(dbParserProject.BuildVariants, 1)
	s.Len(dbParserProject.Tasks, 2)

	s.False(utility.FromBoolPtr(dbVersion.Activated))
	dbBuild, err := build.FindOneId(s.T().Context(), v.BuildIds[0])
	s.NoError(err)
	s.Equal(v.Id, dbBuild.Version)
	s.Len(dbBuild.Tasks, 2)

	dbTasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Len(dbTasks, 2)
}

func (s *CreateVersionFromConfigSuite) TestInvalidConfigErrors() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
  exec_timeout_secs: 3
- name: task2
  exec_timeout_secs: 3
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev}, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)

	dbVersion, err := model.VersionFindOneId(s.ctx, v.Id)
	s.NoError(err)
	s.NotNil(dbVersion)
	s.Require().Len(dbVersion.Errors, 1)
	s.Require().Len(dbVersion.Warnings, 2)
	s.Equal("buildvariant 'bv' must either specify run_on field or have every task specify run_on", dbVersion.Errors[0])
	s.Equal("task 'task1' does not contain any commands", dbVersion.Warnings[0])
	s.Equal("task 'task2' does not contain any commands", dbVersion.Warnings[1])

	s.Equal(evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod)
	dbParserProject, err := model.ParserProjectFindOneByID(s.ctx, s.env.Settings(), dbVersion.ProjectStorageMethod, dbVersion.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbParserProject)
	s.Len(dbParserProject.BuildVariants, 1)
	s.Len(dbParserProject.Tasks, 2)

	dbBuild, err := build.FindOne(s.T().Context(), build.ByVersion(v.Id))
	s.NoError(err)
	s.Nil(dbBuild)

	dbTasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Empty(dbTasks)
}

func (s *CreateVersionFromConfigSuite) TestInvalidAliasErrors() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
- name: task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev, SourceVersion: s.sourceVersion, Alias: "gibberish"}, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)

	dbVersion, err := model.VersionFindOneId(s.ctx, v.Id)
	s.NoError(err)
	s.NotNil(dbVersion)
	s.Require().Len(dbVersion.Errors, 1)
	s.Equal("requested alias 'gibberish' is undefined", dbVersion.Errors[0])

	s.Equal(evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod)
	dbParserProject, err := model.ParserProjectFindOneByID(s.ctx, s.env.Settings(), dbVersion.ProjectStorageMethod, dbVersion.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbParserProject)
	s.Len(dbParserProject.BuildVariants, 1)
	s.Len(dbParserProject.Tasks, 2)

	dbBuild, err := build.FindOne(s.T().Context(), build.ByVersion(v.Id))
	s.NoError(err)
	s.Nil(dbBuild)

	dbTasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Empty(dbTasks)
}

func (s *CreateVersionFromConfigSuite) TestErrorsMerged() {
	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo "test"
- name: task2
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo "test"
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	vErrs := VersionErrors{
		Errors:   []string{"err1"},
		Warnings: []string{"warn1", "warn2"},
	}
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev}, false, &vErrs)
	s.NoError(err)
	s.Require().NotNil(v)

	dbVersion, err := model.VersionFindOneId(s.ctx, v.Id)
	s.NoError(err)
	s.NotNil(dbVersion)
	s.Len(dbVersion.Errors, 2)
	s.Len(dbVersion.Warnings, 2)

	s.Equal(evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod)
	dbParserProject, err := model.ParserProjectFindOneByID(s.ctx, s.env.Settings(), dbVersion.ProjectStorageMethod, dbVersion.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbParserProject)
	s.Len(dbParserProject.BuildVariants, 1)
	s.Len(dbParserProject.Tasks, 2)
}

func (s *CreateVersionFromConfigSuite) TestTransactionAbort() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
- name: task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	s.NotNil(pp)
	//force a duplicate key error with the version
	v := &model.Version{
		Id: makeVersionId(s.ref.Identifier, s.rev.Revision),
	}
	s.NoError(v.Insert(s.ctx))

	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	v, err = CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev, SourceVersion: s.sourceVersion}, false, nil)
	s.Error(err)

	tasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Empty(tasks)
}

func (s *CreateVersionFromConfigSuite) TestWithTaskBatchTime() {
	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
    batchtime: 30
  - name: task2
    cron: "@daily"
  - name: task3
- name: bv2
  batchtime: 15
  display_name: bv2_display
  run_on: d
  tasks:
  - name: task1
tasks:
- name: task1
- name: task2
- name: task3
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	metadata := model.VersionMetadata{Revision: *s.rev}
	now := time.Now()
	tomorrow := now.Add(time.Hour * 24) // next day
	y, m, d := tomorrow.Date()
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, metadata, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)
	s.Empty(v.Errors)

	tasks, err := task.FindAllTaskIDsFromVersion(s.ctx, v.Id)
	s.NoError(err)
	s.Len(tasks, 4)

	s.Len(v.BuildVariants, 2)
	for _, bv := range v.BuildVariants {
		if bv.BuildVariant == "bv" {
			s.Equal(bv.DisplayName, "bv_display")
			s.InDelta(bv.ActivateAt.Unix(), now.Unix(), 1)
			s.Require().Len(bv.BatchTimeTasks, 2)
			for _, t := range bv.BatchTimeTasks {
				if t.TaskName == "task1" { // activate time is now because there isn't a previous task
					s.InDelta(t.ActivateAt.Unix(), now.Unix(), 1)
				} else {
					// ensure that "daily" cron is set for the next day
					ty, tm, td := t.ActivateAt.Date()
					s.Equal(y, ty)
					s.Equal(m, tm)
					s.Equal(d, td)
				}

			}
		}
		if bv.BuildVariant == "bv2" {
			s.Equal(bv.DisplayName, "bv2_display")
			s.False(bv.Activated)
			s.Empty(bv.BatchTimeTasks)
		}
	}
}

func (s *CreateVersionFromConfigSuite) TestCreateVersionItemsBatchtime() {

	for testName, requester := range map[string]string{
		"RepotrackerVersion": evergreen.RepotrackerVersionRequester,
		"TriggerVersion":     evergreen.TriggerRequester,
		"PatchVersion":       evergreen.PatchVersionRequester,
	} {
		s.T().Run(testName, func(t *testing.T) {
			// Test that we correctly use the version create time to determine task
			// batchtimes if the version create time is in the recent past, rather than
			// the task create time.
			configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
    batchtime: 30
  - name: task2
    cron: "@daily"
  - name: task3
- name: bv2
  batchtime: 15
  display_name: bv2_display
  run_on: d
  tasks:
  - name: task1
tasks:
- name: task1
- name: task2
- name: task3
`
			p := &model.Project{}
			pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
			s.NoError(err)
			projectInfo := &model.ProjectInfo{
				Ref:                 s.ref,
				IntermediateProject: pp,
				Project:             p,
			}
			metadata := model.VersionMetadata{Revision: *s.rev}
			versionCreateTime := time.Now().Add(-10 * time.Minute)

			v := &model.Version{
				Id:                  fmt.Sprintf("%s_version", requester),
				CreateTime:          versionCreateTime,
				Revision:            "abc",
				Author:              "me",
				RevisionOrderNumber: 12,
				Owner:               "evergreen-ci",
				Repo:                "evergreen",
				Branch:              "main",
				Requester:           requester,
			}
			s.NoError(createVersionItems(s.ctx, v, metadata, projectInfo, nil))
			tasks, err := task.FindAllTaskIDsFromVersion(s.ctx, v.Id)
			s.NoError(err)
			s.Len(tasks, 4)
			tomorrow := versionCreateTime.Add(time.Hour * 24) // next day
			y, m, d := tomorrow.Date()

			if !evergreen.ShouldConsiderBatchtime(v.Requester) {
				s.Len(v.BuildVariants, 2)
				for _, bv := range v.BuildVariants {
					s.NotEqual(versionCreateTime, bv.ActivateAt, "build variant activation should be based on version create time because not mainline")
					s.Require().Len(bv.BatchTimeTasks, 0, "task cron activation should be empty because not mainline")
				}
			} else {
				for _, bv := range v.BuildVariants {
					if bv.BuildVariant == "bv" {
						s.Equal(versionCreateTime, bv.ActivateAt, "build variant activation should be based on version create time")
						s.Require().Len(bv.BatchTimeTasks, 2)
						for _, t := range bv.BatchTimeTasks {
							if t.TaskName == "task1" {
								s.Equal(versionCreateTime, t.ActivateAt, "task cron activation should be based on version create time because there is no previous task that ran the cron")
							} else {
								// ensure that "daily" cron is set for the next day
								ty, tm, td := t.ActivateAt.Date()
								s.Equal(y, ty)
								s.Equal(m, tm)
								s.Equal(d, td)
							}
						}
					}
					if bv.BuildVariant == "bv2" {
						s.False(bv.Activated)
						s.Empty(bv.BatchTimeTasks)
					}
				}
			}
		})
	}
}

func (s *CreateVersionFromConfigSuite) TestVersionWithDependencies() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv
  display_name: "bv_display"
  run_on: d
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
  depends_on: 
  - name: task2
- name: task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)
	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	alias := model.ProjectAlias{
		Alias:     evergreen.GithubPRAlias,
		ProjectID: s.ref.Id,
		Task:      "task1",
		Variant:   ".*",
	}
	s.NoError(alias.Upsert(s.ctx))
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, model.VersionMetadata{Revision: *s.rev, Alias: evergreen.GithubPRAlias}, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)

	dbVersion, err := model.VersionFindOneId(s.ctx, v.Id)
	s.NoError(err)
	s.NotNil(dbVersion)
	s.Require().NotNil(dbVersion)
	s.Equal(evergreen.VersionCreated, dbVersion.Status)
	s.Equal(s.rev.RevisionMessage, dbVersion.Message)

	s.Require().Len(v.BuildIds, 1)
	dbBuild, err := build.FindOneId(s.T().Context(), v.BuildIds[0])
	s.NoError(err)
	s.Equal(v.Id, dbBuild.Version)
	s.Len(dbBuild.Tasks, 2)

	dbTasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Len(dbTasks, 2)
}

func (s *CreateVersionFromConfigSuite) TestWithAliasAndPatchOptionalDependencyDoesNotCreateDependentTaskAutomatically() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv1
  display_name: bv_display
  run_on: d
  tasks:
  - name: task1
  - name: task2
tasks:
- name: task1
  depends_on:
  - name: task2
    patch_optional: true
- name: task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)

	alias := model.ProjectAlias{
		Alias:     "task1_alias",
		ProjectID: s.ref.Id,
		Task:      "task1",
		Variant:   ".*",
	}
	s.NoError(alias.Upsert(s.ctx))

	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	metadata := model.VersionMetadata{
		Revision: *s.rev,
		Alias:    alias.Alias,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, metadata, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)
	s.Empty(v.Errors)

	tasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Require().Len(tasks, 1)
	s.Equal("task1", tasks[0].DisplayName, "should create task matching alias")
	s.Equal("bv1", tasks[0].BuildVariant, "should create task matching alias in build variant")
	s.Empty(tasks[0].DependsOn, "should not automatically create patch_optional dependency since the task is not created")
}

func (s *CreateVersionFromConfigSuite) TestWithAliasMatchingTaskGroup() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv1
  display_name: bv_display
  run_on: d
  tasks:
  - name: tg1
  - name: task3
tasks:
- name: task1
- name: task2
- name: task3
task_groups:
- name: tg1
  max_hosts: 2
  tasks:
  - task1
  - task2
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)

	alias := model.ProjectAlias{
		Alias:     "tg_alias",
		ProjectID: s.ref.Id,
		Task:      "tg1",
		Variant:   ".*",
	}
	s.NoError(alias.Upsert(s.ctx))

	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	metadata := model.VersionMetadata{
		Revision: *s.rev,
		Alias:    alias.Alias,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, metadata, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)
	s.Empty(v.Errors)

	tasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Require().Len(tasks, 2)
	var foundTask1, foundTask2 bool
	for _, tsk := range tasks {
		switch tsk.DisplayName {
		case "task1":
			s.Equal("bv1", tsk.BuildVariant)
			foundTask1 = true
		case "task2":
			s.Equal("bv1", tsk.BuildVariant)
			foundTask2 = true
		default:
			s.FailNow("unexpected task created", tsk.DisplayName)
		}
	}

	s.True(foundTask1, "should create task1 because it matches alias")
	s.True(foundTask2, "should create task2 because it matches alias")
}

func (s *CreateVersionFromConfigSuite) TestWithAliasAndPatchOptionalDependencyCreatesDependencyIfDependentTaskIsCreated() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configYml := `
buildvariants:
- name: bv1
  display_name: bv_display
  run_on: d
  tasks:
  - name: task1
  - name: task2
  - name: task3
tasks:
- name: task1
  depends_on:
  - name: task2
    patch_optional: true
- name: task2
- name: task3
`
	p := &model.Project{}
	pp, err := model.LoadProjectInto(s.ctx, []byte(configYml), nil, s.ref.Id, p)
	s.NoError(err)

	alias := model.ProjectAlias{
		Alias:     "task_alias",
		ProjectID: s.ref.Id,
		Task:      "(task1)|(task2)",
		Variant:   ".*",
	}
	s.NoError(alias.Upsert(s.ctx))

	projectInfo := &model.ProjectInfo{
		Ref:                 s.ref,
		IntermediateProject: pp,
		Project:             p,
	}
	metadata := model.VersionMetadata{
		Revision: *s.rev,
		Alias:    alias.Alias,
	}
	v, err := CreateVersionFromConfig(s.ctx, projectInfo, metadata, false, nil)
	s.NoError(err)
	s.Require().NotNil(v)
	s.Empty(v.Errors)

	tasks, err := task.Find(ctx, task.ByVersion(v.Id))
	s.NoError(err)
	s.Len(tasks, 2)

	var foundTask1, foundTask2 bool
	var task1DepID, task2ID string
	for _, tsk := range tasks {
		switch tsk.DisplayName {
		case "task1":
			s.Equal("bv1", tsk.BuildVariant)
			s.Require().Len(tsk.DependsOn, 1, "should create patch_optional dependency since the task is created")
			task1DepID = tsk.DependsOn[0].TaskId
			foundTask1 = true
		case "task2":
			s.Equal("bv1", tsk.BuildVariant)
			task2ID = tsk.Id
			foundTask2 = true
		default:
			s.FailNow("unexpected task created", tsk.DisplayName)
		}
	}

	s.True(foundTask1, "should create task1 because it matches alias")
	s.True(foundTask2, "should create task2 because it matches alias")
	s.Equal(task1DepID, task2ID, "task1 should depend on task2 even with patch_optional dependency because task2 was created")
}

func TestCreateManifest(t *testing.T) {
	assert := assert.New(t)
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings)
	require.NoError(t, db.ClearCollections(model.VersionCollection, model.ProjectRefCollection, model.ProjectVarsCollection), fakeparameter.Collection)
	// with a revision from 5/31/15
	v := model.Version{
		Id:         "aaaaaaaaaaff001122334455",
		Revision:   "1bb42195fd415f144abbae509a5d5bef80d829b7",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
	}

	patchVersion := model.Version{
		Id:         "aaaaaaaaaaff001122334456",
		Revision:   "1bb42195fd415f144abbae509a5d5bef80d829b7",
		Identifier: "proj",
		Requester:  evergreen.GithubPRRequester,
	}

	patch := patch.Patch{
		Id: bson.ObjectIdHex("aaaaaaaaaaff001122334456"),
	}
	require.NoError(t, patch.Insert(t.Context()))

	// no revision specified
	proj := model.Project{
		Identifier: "proj",
		Modules: []model.Module{
			{
				Name:   "module1",
				Owner:  "evergreen-ci",
				Repo:   "sample",
				Branch: "${var1}",
			},
		},
	}
	projRef := &model.ProjectRef{
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "main",
		Id:     "project1",
	}
	require.NoError(t, projRef.Insert(t.Context()))

	projVars := model.ProjectVars{
		Id: "project1",
		Vars: map[string]string{
			"var1": "main",
		},
	}
	require.NoError(t, projVars.Insert(t.Context()))

	manifest, err := model.CreateManifest(t.Context(), &v, proj.Modules, projRef)
	assert.NoError(err)
	assert.Equal(v.Id, manifest.Id)
	assert.Equal(v.Revision, manifest.Revision)
	assert.Len(manifest.Modules, 1)
	module, ok := manifest.Modules["module1"]
	assert.True(ok)
	assert.Equal("sample", module.Repo)
	assert.Equal("main", module.Branch)
	// the most recent module commit as of the version's revision (from 5/30/15)
	assert.Equal("b27779f856b211ffaf97cbc124b7082a20ea8bc0", module.Revision)

	proj.Modules[0].AutoUpdate = true
	manifest, err = model.CreateManifest(t.Context(), &patchVersion, proj.Modules, projRef)
	assert.NoError(err)
	assert.Equal(patchVersion.Id, manifest.Id)
	assert.Equal(patchVersion.Revision, manifest.Revision)
	assert.Len(manifest.Modules, 1)
	module, ok = manifest.Modules["module1"]
	assert.True(ok)
	assert.Equal("sample", module.Repo)
	assert.Equal("main", module.Branch)
	// a patch version should use the most recent module commit as of the current time
	assert.NotNil(module.Revision)
	assert.NotEqual("b27779f856b211ffaf97cbc124b7082a20ea8bc0", module.Revision)

	// revision specified
	hash := "cf46076567e4949f9fc68e0634139d4ac495c89b"
	proj = model.Project{
		Identifier: "proj",
		Modules: []model.Module{
			{
				Name:   "module1",
				Owner:  "evergreen-ci",
				Repo:   "sample",
				Branch: "main",
				Ref:    hash,
			},
		},
	}
	manifest, err = model.CreateManifest(t.Context(), &v, proj.Modules, projRef)
	assert.NoError(err)
	assert.Equal(v.Id, manifest.Id)
	assert.Equal(v.Revision, manifest.Revision)
	assert.Len(manifest.Modules, 1)
	module, ok = manifest.Modules["module1"]
	assert.True(ok)
	assert.Equal("sample", module.Repo)
	assert.Equal("main", module.Branch)
	assert.Equal(hash, module.Revision)
	assert.NotEmpty(module.URL)

	// invalid revision
	hash = "1234"
	proj = model.Project{
		Identifier: "proj",
		Modules: []model.Module{
			{
				Name:   "module1",
				Repo:   "git@github.com:evergreen-ci/sample.git",
				Branch: "main",
				Ref:    hash,
			},
		},
	}
	_, err = model.CreateManifest(t.Context(), &v, proj.Modules, projRef)
	assert.Contains(err.Error(), "No commit found for SHA")
}

func TestShellVersionFromRevisionGitTags(t *testing.T) {
	assert.NoError(t, db.ClearCollections(user.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// triggered from yaml
	metadata := model.VersionMetadata{
		RemotePath: "releases.yml",
		Revision: model.Revision{
			AuthorID:        "regina.phalange",
			Author:          "Regina Phalange",
			AuthorEmail:     "not-fake@email.com",
			RevisionMessage: "EVG-1234 good version",
			Revision:        "1234",
		},
		GitTag: model.GitTag{
			Tag:    "release",
			Pusher: "release-bot",
		},
	}
	pRef := &model.ProjectRef{
		Id:                    primitive.NewObjectID().Hex(),
		Identifier:            "my-project",
		GitTagAuthorizedUsers: []string{"release-bot", "not-release-bot"},
		GitTagVersionsEnabled: utility.TruePtr(),
	}

	assert.NoError(t, evergreen.UpdateConfig(ctx, testutil.TestConfig()))
	v, err := ShellVersionFromRevision(t.Context(), pRef, metadata)
	assert.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, evergreen.GitTagRequester, v.Requester)
	assert.Equal(t, metadata.GitTag.Tag, v.TriggeredByGitTag.Tag)
	assert.Equal(t, metadata.GitTag.Pusher, v.TriggeredByGitTag.Pusher)
	assert.Equal(t, metadata.RemotePath, v.RemotePath)
	assert.Contains(t, v.Id, "my_project_release_")
	assert.Equal(t, "Triggered From Git Tag 'release': EVG-1234 good version", v.Message)
	// When no Evergreen user exists for the pusher, we use the author of the version that was pushed to.
	assert.Equal(t, metadata.Revision.AuthorID, v.AuthorID)
	assert.Equal(t, metadata.Revision.Author, v.Author)
	assert.Equal(t, metadata.Revision.AuthorEmail, v.AuthorEmail)

	usr := &user.DBUser{
		Id:           "release-bot-boi",
		DispName:     "Release Bot",
		EmailAddress: "release-is-cool@progress.com",
		OnlyAPI:      true,
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				LastKnownAs: "release-bot",
				UID:         12,
			},
		},
	}
	assert.NoError(t, usr.Insert(t.Context()))

	v, err = ShellVersionFromRevision(t.Context(), pRef, metadata)
	assert.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, evergreen.GitTagRequester, v.Requester)
	assert.Equal(t, metadata.GitTag.Tag, v.TriggeredByGitTag.Tag)
	assert.Equal(t, metadata.GitTag.Pusher, v.TriggeredByGitTag.Pusher)
	assert.Equal(t, metadata.RemotePath, v.RemotePath)
	assert.Contains(t, v.Id, "my_project_release_")
	assert.Equal(t, "Triggered From Git Tag 'release': EVG-1234 good version", v.Message)
	// When an Evergreen user does exist for the pusher, we use this for the author.
	assert.Equal(t, usr.Id, v.AuthorID)
	assert.Equal(t, usr.DisplayName(), v.Author)
	assert.Equal(t, usr.Email(), v.AuthorEmail)
}

func TestCreateVersionItemsPathFiltering(t *testing.T) {
	testCases := []struct {
		name            string
		metadata        model.VersionMetadata
		ignoredVariants []string
	}{
		{
			name: "FrontendFilesOnly",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"frontend/src/app.js", "frontend/package.json"},
			},
			ignoredVariants: []string{"backend"},
		},
		{
			name: "BackendFilesOnly",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"backend/src/server.go", "backend/config.yml"},
			},
			ignoredVariants: []string{"frontend"},
		},
		{
			name: "SharedFiles",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"shared/utils.js", "shared/constants.go"},
			},
			ignoredVariants: []string{},
		},
		{
			name: "NoChangedFiles",
			metadata: model.VersionMetadata{
				ChangedFiles: nil,
			},
			ignoredVariants: []string{},
		},
		{
			name: "NonMatchingFiles",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"docs/README.md", "scripts/deploy.sh"},
			},
			ignoredVariants: []string{"frontend", "backend"},
		},
		{
			name: "GoFileInNonBackendLocation",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"tools/migrate/main.go", "scripts/deploy.go"},
			},
			ignoredVariants: []string{"frontend"},
		},
		{
			name: "MarkdownFileChanged",
			metadata: model.VersionMetadata{
				ChangedFiles: []string{"docs/README.md", "CHANGELOG.md"},
			},
			ignoredVariants: []string{"frontend", "backend", "non_docs"},
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Clear collections before running tests
			require.NoError(t, db.ClearCollections(model.VersionCollection, build.Collection, task.Collection))

			// Test configuration with build variants that have different path patterns
			configYml := `
buildvariants:
- name: frontend
  display_name: Frontend
  run_on: d
  paths:
  - "frontend/**"
  - "shared/**"
  tasks:
  - name: frontend_test
  - name: special_task
    activate: true
- name: backend
  display_name: Backend
  run_on: d
  paths:
  - "backend/**"
  - "shared/**"
  - "**/*.go"
  - "go.mod"
  tasks:
  - name: backend_test
- name: frontend-special-cron
  # this variant is never ignored on mainline due to cron, despite path filtering.
  cron: 0 0 * * *
  paths:
  - "frontend/**"
  - "shared/**"
  tasks:
  - name: frontend_test
- name: non_docs
  display_name: Non-Documentation
  run_on: d
  paths:
  - "!**/*.md"
  tasks:
  - name: non_docs_test
- name: all_files
  display_name: All Files
  run_on: d
  # No paths specified - should always run
  tasks:
  - name: integration_test
tasks:
- name: frontend_test
- name: backend_test
- name: non_docs_test
- name: integration_test
- name: special_task
`

			projectRef := &model.ProjectRef{
				Owner:      "evergreen-ci",
				Repo:       "evergreen",
				Branch:     "main",
				Id:         "project1",
				Identifier: "project1",
			}

			p := &model.Project{}
			pp, err := model.LoadProjectInto(ctx, []byte(configYml), nil, projectRef.Id, p)
			require.NoError(t, err)

			projectInfo := &model.ProjectInfo{
				Ref:                 projectRef,
				IntermediateProject: pp,
				Project:             p,
			}

			v := &model.Version{
				Id:                  fmt.Sprintf("test_version_%s_%d", tc.name, time.Now().UnixNano()),
				CreateTime:          time.Now(),
				Revision:            fmt.Sprintf("rev%d", i+1),
				Author:              "test_author",
				Activated:           utility.TruePtr(),
				RevisionOrderNumber: i + 1,
				Owner:               "test_owner",
				Repo:                "test_repo",
				Branch:              "main",
				Requester:           evergreen.RepotrackerVersionRequester,
			}

			err = createVersionItems(ctx, v, tc.metadata, projectInfo, nil)
			require.NoError(t, err)

			ignoredVariants := []string{}
			for _, bv := range v.BuildVariants {
				if utility.IsZeroTime(bv.ActivateAt) { // This should be set if the variant is ignored due to path filtering.
					ignoredVariants = append(ignoredVariants, bv.BuildVariant)
					if bv.BuildVariant == "frontend" { // Ensure that the task that overrides activation is given an activation time.
						require.Len(t, bv.BatchTimeTasks, 1)
						assert.Equal(t, bv.BatchTimeTasks[0].TaskName, "special_task")
					}
				}
			}

			// Verify that all expected variants are ignored.
			for _, ignoredVariant := range tc.ignoredVariants {
				assert.Contains(t, ignoredVariants, ignoredVariant, "Expected variant '%s' to be ignored", ignoredVariant)
			}
			// Verify that only expected variants are ignored.
			assert.Len(t, ignoredVariants, len(tc.ignoredVariants), "Expected exactly %d variants to be ignored, got %d", len(tc.ignoredVariants), len(ignoredVariants))
		})
	}

}
