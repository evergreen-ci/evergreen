package units

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type PatchIntentUnitsSuite struct {
	sender         *send.InternalSender
	env            *mock.Environment
	originalEnv    evergreen.Environment
	originalConfig *evergreen.Settings
	suiteCtx       context.Context
	cancel         context.CancelFunc
	ctx            context.Context

	repo            string
	headRepo        string
	prNumber        int
	user            string
	hash            string
	baseHash        string
	diffURL         string
	desc            string
	project         string
	variants        []string
	tasks           []string
	githubPatchData thirdparty.GithubPatch

	suite.Suite
}

func TestPatchIntentUnitsSuite(t *testing.T) {
	s := new(PatchIntentUnitsSuite)
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)
	suite.Run(t, s)
}

func (s *PatchIntentUnitsSuite) TearDownSuite() {
	s.cancel()
}

func (s *PatchIntentUnitsSuite) TearDownTest() {
	if s.originalEnv != nil {
		evergreen.SetEnvironment(s.originalEnv)
	}
	if s.originalConfig != nil {
		s.NoError(evergreen.UpdateConfig(s.ctx, s.originalConfig))
	}
}

func (s *PatchIntentUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())
	s.Require().NoError(s.env.Configure(s.ctx))

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	s.NotNil(s.env.Settings())

	// This setup has to both:
	// 1. update the admin settings in the DB and
	// 2. set the global environment.
	//
	// Both are necessary because deep down in the logic to set up GitHub merge
	// queue subscriptions, it ignores the job's environment and instead:
	// 1. loads the GitHub app settings directly from the DB to check branch
	//       protection settings (so it's using the DB settings) and
	// 2. calls evergreen.GetEnvironment directly to send a pending status to
	//       GitHub (so it's using the global testing environment).
	s.originalEnv = evergreen.GetEnvironment()
	originalConfig, err := evergreen.GetConfig(s.ctx)
	s.Require().NoError(err)
	s.originalConfig = originalConfig
	evergreen.SetEnvironment(s.env)
	s.NoError(evergreen.UpdateConfig(s.ctx, s.env.Settings()))

	s.NoError(db.ClearCollections(evergreen.ConfigCollection, task.Collection, model.ProjectVarsCollection,
		model.ParserProjectCollection, model.VersionCollection, user.Collection, model.ProjectRefCollection,
		model.ProjectAliasCollection, patch.Collection, patch.IntentCollection, event.SubscriptionsCollection, distro.Collection))
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	s.NoError((&model.ProjectRef{
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Id:               "mci",
		Enabled:          true,
		PatchingDisabled: utility.FalsePtr(),
		Branch:           "main",
		RemotePath:       "self-tests.yml",
		PRTestingEnabled: utility.TruePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{
				Alias:          "patch-alias",
				ChildProject:   "childProj",
				TaskSpecifiers: []patch.TaskSpecifier{{PatchAlias: "childProj-patch-alias"}},
			},
		},
	}).Insert())

	s.NoError((&model.ProjectRef{
		Owner:            "evergreen-ci",
		Repo:             "commit-queue-sandbox",
		Id:               "commit-queue-sandbox",
		Enabled:          true,
		PatchingDisabled: utility.FalsePtr(),
		Branch:           "main",
		RemotePath:       "evergreen.yml",
		PRTestingEnabled: utility.TruePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		OldestAllowedMergeBase: "536cde7f7b29f7e117371a48a3e59540a44af1ac",
	}).Insert())

	s.NoError((&model.ProjectRef{
		Id:         "childProj",
		Identifier: "childProj",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "main",
		Enabled:    true,
		RemotePath: "self-tests.yml",
	}).Insert())

	s.NoError((&user.DBUser{
		Id: evergreen.GithubPatchUser,
	}).Insert())

	s.NoError((&model.ProjectVars{
		Id: "mci",
	}).Insert())

	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     evergreen.GithubPRAlias,
		Variant:   "ubuntu.*",
		Task:      "dist.*",
	}).Upsert())
	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     evergreen.GithubPRAlias,
		Variant:   "race.*",
		Task:      "dist.*",
	}).Upsert())
	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     "doesntexist",
		Variant:   "fake",
		Task:      "fake",
	}).Upsert())
	s.NoError((&model.ProjectAlias{
		ProjectID: "commit-queue-sandbox",
		Alias:     evergreen.CommitQueueAlias,
		Variant:   "^ubuntu2004$",
		Task:      "^bynntask$",
	}).Upsert())

	s.NoError((&distro.Distro{Id: "ubuntu1604-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "ubuntu1604-build"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "archlinux-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "archlinux-build"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "windows-64-vs2015-small"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "rhel71-power8-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "rhel72-zseries-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "ubuntu1604-arm64-small"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "rhel62-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "rhel70-small"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "rhel62-small"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "linux-64-amzn-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "debian81-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "debian71-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "ubuntu1404-test"}).Insert(s.ctx))
	s.NoError((&distro.Distro{Id: "macos-1012"}).Insert(s.ctx))

	s.repo = "hadjri/evergreen"
	s.headRepo = "tychoish/evergreen"
	s.prNumber = 5290
	s.user = evergreen.GithubPatchUser
	s.hash = "8b9b7ee42ef46d40e391910e3afd00e187a9dae8"
	s.baseHash = "7c38f3f63c05675329518c148d3a176e1da6ec2d"
	s.diffURL = "https://github.com/evergreen-ci/evergreen/pull/5290.diff"
	s.githubPatchData = thirdparty.GithubPatch{
		PRNumber:   448,
		BaseOwner:  "evergreen-ci",
		BaseRepo:   "evergreen",
		BaseBranch: "main",
		HeadOwner:  "richardsamuels",
		HeadRepo:   "evergreen",
		HeadHash:   "something",
		Author:     "richardsamuels",
	}
	s.desc = "Test!"
	s.project = "mci"
	s.variants = []string{"ubuntu1604", "ubuntu1604-arm64", "ubuntu1604-debug", "race-detector"}
	s.tasks = []string{"dist", "dist-test"}

	s.NoError((&model.Version{
		Identifier: s.project,
		CreateTime: time.Now(),
		Revision:   s.hash,
		Requester:  evergreen.RepotrackerVersionRequester,
	}).Insert())

	factory, err := registry.GetJobFactory(patchIntentJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, patchIntentJobName)
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithNoTasksAndVariants() {
	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:         s.user,
		Project:      s.project,
		BaseGitHash:  s.hash,
		PatchContent: string(body),
		Description:  s.desc,
		Finalize:     true,
		Alias:        "doesntexist",
	})
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(s.ctx, patchDoc)
	s.Require().Error(err)
	s.Equal("patch has no build variants or tasks", err.Error())
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithBadAlias() {
	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:         s.user,
		Project:      s.project,
		BaseGitHash:  s.hash,
		PatchContent: string(body),
		Description:  s.desc,
		Finalize:     true,
		Alias:        "typo",
	})
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(s.ctx, patchDoc)
	s.Require().Error(err)
	s.Equal("alias 'typo' could not be found on project 'mci'", err.Error())
}

func (s *PatchIntentUnitsSuite) TestCantFinishCommitQueuePatchWithNoTasksAndVariants() {
	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	s.NoError(db.ClearCollections(model.ProjectAliasCollection))

	s.NoError((&model.ProjectAlias{
		ProjectID: s.project,
		Alias:     evergreen.CommitQueueAlias,
	}).Upsert())

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:         s.user,
		Project:      s.project,
		BaseGitHash:  s.hash,
		PatchContent: string(body),
		Description:  s.desc,
		Alias:        evergreen.CommitQueueAlias,
	})
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(s.ctx, patchDoc)
	s.Require().Error(err)
	s.Equal("patch has no build variants or tasks", err.Error())
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithDisabledCommitQueue() {
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	headRef := "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
	orgName := "evergreen-ci"
	repoName := "commit-queue-sandbox"
	headSHA := "foo"
	baseSHA := "bar"

	disabledMergeQueueProject := model.ProjectRef{
		Id: repoName,
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.FalsePtr(),
		},
	}
	s.Require().NoError(disabledMergeQueueProject.Upsert())

	org := github.Organization{
		Login: &orgName,
	}
	repo := github.Repository{
		Name: &repoName,
	}
	mg := github.MergeGroup{
		HeadSHA: &headSHA,
		HeadRef: &headRef,
		BaseSHA: &baseSHA,
	}
	mge := github.MergeGroupEvent{
		MergeGroup: &mg,
		Org:        &org,
		Repo:       &repo,
	}
	intent, err := patch.NewGithubMergeIntent("id", "auto", &mge)

	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)

	patchDoc := intent.NewPatch()
	s.Error(j.finishPatch(s.ctx, patchDoc))
	s.NotEmpty(j.gitHubError)
	s.Equal(j.gitHubError, commitQueueDisabled)
}

func (s *PatchIntentUnitsSuite) TestSetToPreviousPatchDefinition() {
	patchId := mgobson.NewObjectId().Hex()
	previousPatchDoc := &patch.Patch{
		Id:         patch.NewId(patchId),
		Activated:  true,
		Status:     evergreen.VersionFailed,
		Project:    s.project,
		CreateTime: time.Now(),
		Author:     "me",
		Version:    "v1",
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"t1", "t2", "tgt1", "tgt2", "tgt3", "tgt4", "not_activated"},
			},
			{
				Variant:      "bv2",
				DisplayTasks: []patch.DisplayTask{{Name: "dt1", ExecTasks: []string{"et1"}}},
				Tasks:        []string{"et1"},
			},
			{
				Variant:      "bv3",
				DisplayTasks: []patch.DisplayTask{{Name: "dt2", ExecTasks: []string{"et2"}}},
				Tasks:        []string{"et2"},
			},
		},
		Tasks:         []string{"t1", "t2", "tgt1", "tgt2", "tgt3", "tgt4", "dt1", "et1", "dt2", "et2", "not_activated"},
		BuildVariants: []string{"bv1", "bv2", "bv3"},
		RegexTasks:    []string{"1$"},
	}
	s.NoError((previousPatchDoc).Insert())

	reusePatchId := mgobson.NewObjectId().Hex()
	reusePatchDoc := &patch.Patch{
		Id:         patch.NewId(reusePatchId),
		Activated:  true,
		Status:     evergreen.VersionFailed,
		Project:    s.project,
		CreateTime: time.Now().Add(-time.Hour),
		Author:     "me",
		Version:    "diffVersion",
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"diffTask1", "diffTask2"},
			},
		},
		Tasks:         []string{"diffTask1", "diffTask2"},
		BuildVariants: []string{"bv1"},
	}
	s.NoError((reusePatchDoc).Insert())

	project := model.Project{Identifier: s.project, BuildVariants: model.BuildVariants{
		{
			Name: "bv1",
			Tasks: []model.BuildVariantTaskUnit{
				{Name: "t1", Variant: "bv1"},
				{Name: "t2", Variant: "bv1"},
				{Name: "tg", Variant: "bv1", IsGroup: true},
				{Name: "tg2", Variant: "bv1", IsGroup: true},
				{Name: "diffTask1", Variant: "bv1"},
				{Name: "diffTask2", Variant: "bv1"},
			},
		},
		{
			Name:         "bv2",
			DisplayTasks: []patch.DisplayTask{{Name: "dt1", ExecTasks: []string{"et1"}}},
			Tasks: []model.BuildVariantTaskUnit{
				{Name: "et1", Variant: "bv2"},
			},
		},
		{
			Name:         "bv3",
			DisplayTasks: []patch.DisplayTask{{Name: "dt2", ExecTasks: []string{"et2"}}},
		},
	},
		TaskGroups: []model.TaskGroup{
			{
				Name:     "tg",
				Tasks:    []string{"tgt1", "tgt2"},
				MaxHosts: 1,
			},
			{
				Name:  "tg2",
				Tasks: []string{"tgt3", "tgt4"},
			},
		},
	}
	for testName, testCase := range map[string]func(*patchIntentProcessor, *patch.Patch){
		"previous/reuse": func(j *patchIntentProcessor, currentPatchDoc *patch.Patch) {
			err := j.setToPreviousPatchDefinition(currentPatchDoc, &project, "", false)
			s.NoError(err)
			sort.Strings(previousPatchDoc.Tasks)
			sort.Strings(previousPatchDoc.BuildVariants)
			sort.Strings(currentPatchDoc.Tasks)
			sort.Strings(currentPatchDoc.BuildVariants)

			s.Equal(previousPatchDoc.BuildVariants, currentPatchDoc.BuildVariants)
			s.Equal([]string{"et1", "t1", "t2", "tgt1", "tgt2", "tgt4"}, currentPatchDoc.Tasks)

		},
		"previous/reuse failed": func(j *patchIntentProcessor, currentPatchDoc *patch.Patch) {
			err := j.setToPreviousPatchDefinition(currentPatchDoc, &project, "", true)
			s.NoError(err)
			sort.Strings(previousPatchDoc.BuildVariants)
			sort.Strings(currentPatchDoc.BuildVariants)
			s.Equal(previousPatchDoc.BuildVariants, currentPatchDoc.BuildVariants)
			sort.Strings(currentPatchDoc.Tasks)
			s.Equal([]string{"et1", "t1", "tgt1", "tgt2", "tgt4"}, currentPatchDoc.Tasks)
		},
		"specific patch/reuse": func(j *patchIntentProcessor, currentPatchDoc *patch.Patch) {
			err := j.setToPreviousPatchDefinition(currentPatchDoc, &project, reusePatchId, false)
			s.NoError(err)

			s.Equal(currentPatchDoc.BuildVariants, reusePatchDoc.BuildVariants)
			s.Equal(currentPatchDoc.Tasks, []string{"diffTask1", "diffTask2"})

		},
		"specific patch/reuse failed": func(j *patchIntentProcessor, currentPatchDoc *patch.Patch) {
			err := j.setToPreviousPatchDefinition(currentPatchDoc, &project, reusePatchId, true)
			s.NoError(err)
			s.Equal(currentPatchDoc.BuildVariants, reusePatchDoc.BuildVariants)
			s.Equal(currentPatchDoc.Tasks, []string{"diffTask1"})
		},
	} {
		s.NoError(db.ClearCollections(task.Collection))
		t1 := task.Task{
			Id:           "t1",
			DisplayName:  "t1",
			Version:      "v1",
			BuildVariant: "bv1",
			Status:       evergreen.TaskFailed,
			Activated:    true,
		}
		t2 := task.Task{
			Id:           "t2",
			DisplayName:  "t2",
			Version:      "v1",
			BuildVariant: "bv1",
			Status:       evergreen.TaskSucceeded,
			Activated:    true,
		}
		tgt1 := task.Task{
			Id:                "tgt1",
			DisplayName:       "tgt1",
			Version:           "v1",
			BuildVariant:      "bv1",
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			Status:            evergreen.TaskSucceeded,
			Activated:         true,
		}
		tgt2 := task.Task{
			Id:                "tgt2",
			DisplayName:       "tgt2",
			Version:           "v1",
			BuildVariant:      "bv1",
			TaskGroup:         "tg",
			TaskGroupMaxHosts: 1,
			Status:            evergreen.TaskFailed,
			Activated:         true,
		}
		tgt3 := task.Task{
			Id:           "tgt3",
			DisplayName:  "tgt3",
			Version:      "v1",
			BuildVariant: "bv1",
			TaskGroup:    "tg2",
			Status:       evergreen.TaskSucceeded,
			Activated:    false,
		}
		tgt4 := task.Task{
			Id:           "tgt4",
			DisplayName:  "tgt4",
			Version:      "v1",
			BuildVariant: "bv1",
			TaskGroup:    "tg2",
			Status:       evergreen.TaskFailed,
			Activated:    true,
		}
		notActivated := task.Task{
			Id:           "not_activated",
			DisplayName:  "not_activated",
			Version:      "v1",
			BuildVariant: "bv1",
			Status:       evergreen.TaskInactive,
			Activated:    false,
		}
		dt1 := task.Task{
			Id:             "dt1",
			DisplayName:    "dt1",
			Version:        "v1",
			BuildVariant:   "bv2",
			DisplayOnly:    true,
			ExecutionTasks: []string{"et1"},
			Status:         evergreen.TaskFailed,
			Activated:      true,
		}
		et1 := task.Task{
			Id:            "et1",
			DisplayName:   "et1",
			Version:       "v1",
			BuildVariant:  "bv2",
			DisplayTaskId: utility.ToStringPtr("dt1"),
			Status:        evergreen.TaskFailed,
			Activated:     true,
		}
		dt2 := task.Task{
			Id:             "dt2",
			DisplayName:    "dt2",
			Version:        "v1",
			BuildVariant:   "bv3",
			DisplayOnly:    true,
			ExecutionTasks: []string{"et2"},
			Status:         evergreen.TaskSucceeded,
			Activated:      true,
		}
		et2 := task.Task{
			Id:            "et2",
			DisplayName:   "et2",
			Version:       "v1",
			BuildVariant:  "bv3",
			DisplayTaskId: utility.ToStringPtr("dt2"),
			Status:        evergreen.TaskSucceeded,
			Activated:     false,
		}
		diffTask1 := task.Task{
			Id:           "diffTask1",
			DisplayName:  "diffTask1",
			Version:      "diffVersion",
			BuildVariant: "bv1",
			Status:       evergreen.TaskFailed,
			Activated:    true,
		}
		diffTask2 := task.Task{
			Id:           "diffTask2",
			DisplayName:  "diffTask2",
			Version:      "diffVersion",
			BuildVariant: "bv1",
			Status:       evergreen.TaskSucceeded,
			Activated:    true,
		}
		s.NoError(db.InsertMany(task.Collection, t1, t2, tgt1, tgt2, tgt3, tgt4,
			dt1, et1, dt2, et2, notActivated, diffTask1, diffTask2))

		intent, err := patch.NewCliIntent(patch.CLIIntentParams{
			User:             "me",
			Project:          s.project,
			BaseGitHash:      s.hash,
			Description:      s.desc,
			RepeatDefinition: true,
		})
		s.NoError(err)
		j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
		j.user = &user.DBUser{Id: "me"}

		currentPatchDoc := intent.NewPatch()
		s.Run(testName, func() {
			testCase(j, currentPatchDoc)
		})
	}
}

func (s *PatchIntentUnitsSuite) TestBuildTasksAndVariantsWithRepeatFailed() {
	patchId := "aaaaaaaaaaff001122334455"
	tasks := []task.Task{
		{
			Id:           "t1",
			Activated:    true,
			DependsOn:    []task.Dependency{{TaskId: "t3", Status: evergreen.TaskSucceeded}},
			Version:      patchId,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Status:       evergreen.TaskFailed,
			Project:      s.project,
			DisplayName:  "t1",
		},
		{
			Id:           "t2",
			Activated:    true,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Version:      patchId,
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t2",
		},
		{
			Id:           "t3",
			Activated:    true,
			DependsOn:    []task.Dependency{{TaskId: "t4", Status: evergreen.TaskSucceeded}},
			Version:      patchId,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t3",
		},
		{
			Id:           "t4",
			Activated:    true,
			Version:      patchId,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t4",
		},
	}

	for _, t := range tasks {
		s.NoError(t.Insert())
	}

	previousPatchDoc := &patch.Patch{
		Id:            patch.NewId(patchId),
		Activated:     true,
		Status:        evergreen.VersionFailed,
		Project:       s.project,
		CreateTime:    time.Now(),
		Author:        s.user,
		Version:       patchId,
		Tasks:         []string{"t1", "t2", "t3", "t4"},
		BuildVariants: []string{"bv1"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"t1", "t2", "t3", "t4"},
			},
		},
	}
	s.NoError((previousPatchDoc).Insert())

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:        s.user,
		Project:     s.project,
		BaseGitHash: s.hash,
		Description: s.desc,
		// --repeat-failed flag
		RepeatFailed: true,
	})
	s.NoError(err)
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: s.user}

	project := model.Project{
		Identifier: s.project,
		BuildVariants: model.BuildVariants{
			{
				Name: "bv1",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "t1",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskSucceeded},
						},
					},
					{
						Name:    "t2",
						Variant: "bv1",
					},
					{
						Name:    "t3",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t4",
						Variant: "bv1",
					}},
			},
		},
		Tasks: []model.ProjectTask{
			{Name: "t1"},
			{Name: "t2"},
			{Name: "t3"},
			{Name: "t4"},
		},
	}

	currentPatchDoc := intent.NewPatch()

	s.NoError(err)

	err = j.buildTasksAndVariants(currentPatchDoc, &project)
	s.NoError(err)
	sort.Strings(currentPatchDoc.Tasks)
	s.Equal([]string{"t1", "t3", "t4"}, currentPatchDoc.Tasks)
}

func (s *PatchIntentUnitsSuite) TestBuildTasksAndVariantsWithReuse() {
	patchId := "aaaaaaaaaaff001122334455"
	tasks := []task.Task{
		{
			Id:           "t1",
			Activated:    true,
			DependsOn:    []task.Dependency{{TaskId: "t3", Status: evergreen.TaskSucceeded}},
			Version:      patchId,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Status:       evergreen.TaskFailed,
			Project:      s.project,
			DisplayName:  "t1",
		},
		{
			Id:           "t2",
			Activated:    true,
			BuildVariant: "bv2",
			BuildId:      "b0",
			Version:      patchId,
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t2",
		},
		{
			Id:           "t3",
			Activated:    true,
			BuildVariant: "bv2",
			BuildId:      "b0",
			Version:      patchId,
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t3",
		},
		{
			Id:           "t4",
			Activated:    true,
			BuildVariant: "bv2",
			BuildId:      "b0",
			Version:      patchId,
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t4",
		},
	}

	for _, t := range tasks {
		s.NoError(t.Insert())
	}

	previousPatchDoc := &patch.Patch{
		Id:            patch.NewId(patchId),
		Activated:     true,
		Status:        evergreen.VersionFailed,
		Project:       s.project,
		CreateTime:    time.Now(),
		Author:        s.user,
		Version:       patchId,
		Tasks:         []string{"t1", "t2", "t3,", "t4"},
		BuildVariants: []string{"bv1", "bv2"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"t1", "t3", "t4"},
			},
			{
				Variant: "bv2",
				Tasks:   []string{"t2"},
			},
		},
	}
	s.NoError((previousPatchDoc).Insert())

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:        s.user,
		Project:     s.project,
		BaseGitHash: s.hash,
		Description: s.desc,
		// --reuse flag
		RepeatDefinition: true,
	})
	s.NoError(err)
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: s.user}

	project := model.Project{
		Identifier: s.project,
		BuildVariants: model.BuildVariants{
			{
				Name: "bv1",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "t1",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t2",
						Variant: "bv1",
					},
					{
						Name:    "t3",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t4",
						Variant: "bv1",
					}},
			},
			{
				Name: "bv2",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "t1",
						Variant: "bv2",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t2",
						Variant: "bv2",
					},
					{
						Name:    "t3",
						Variant: "bv2",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t4",
						Variant: "bv2",
					}},
			},
		},
		Tasks: []model.ProjectTask{
			{Name: "t1"},
			{Name: "t2"},
			{Name: "t3"},
			{Name: "t4"},
		},
	}

	currentPatchDoc := intent.NewPatch()

	s.NoError(err)

	// test --reuse
	err = j.buildTasksAndVariants(currentPatchDoc, &project)
	s.NoError(err)
	sort.Strings(currentPatchDoc.Tasks)
	s.Equal([]string{"t1", "t2", "t3", "t4"}, currentPatchDoc.Tasks)

	// ensure tasks are not duplicated to buildvariants when they are not in the previous patch
	s.NotContains("bv1", currentPatchDoc.VariantsTasks[0])
	s.NotContains("bv2", currentPatchDoc.VariantsTasks[1])

	s.Equal([]string{"t1", "t3", "t4"}, currentPatchDoc.VariantsTasks[0].Tasks)
	s.Equal([]string{"t2"}, currentPatchDoc.VariantsTasks[1].Tasks)
}

func (s *PatchIntentUnitsSuite) TestBuildTasksAndVariantsWithReusePatchId() {
	earlierPatchId := mgobson.NewObjectId().Hex()
	tasks := []task.Task{
		{
			Id:           "t1",
			Activated:    true,
			DependsOn:    []task.Dependency{{TaskId: "t3", Status: evergreen.TaskSucceeded}},
			Version:      earlierPatchId,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Status:       evergreen.TaskFailed,
			Project:      s.project,
			DisplayName:  "t1",
		},
		{
			Id:           "t2",
			Activated:    true,
			BuildVariant: "bv1",
			BuildId:      "b0",
			Version:      earlierPatchId,
			Status:       evergreen.TaskSucceeded,
			Project:      s.project,
			DisplayName:  "t2",
		},
		{
			Id:           "t3",
			Activated:    true,
			Version:      earlierPatchId,
			DependsOn:    []task.Dependency{{TaskId: "t4", Status: evergreen.TaskSucceeded}},
			BuildVariant: "bv1",
			BuildId:      "b00",
			Status:       evergreen.TaskFailed,
			Project:      s.project,
			DisplayName:  "t3",
		},
		{
			Id:           "t4",
			Activated:    true,
			BuildVariant: "bv1",
			BuildId:      "b00",
			Version:      earlierPatchId,
			Status:       evergreen.TaskFailed,
			Project:      s.project,
			DisplayName:  "t4",
		},
	}

	for _, t := range tasks {
		s.NoError(t.Insert())
	}

	// Add a more recent patch to ensure that it uses the passed
	// in patchId and not just the latest.
	prevPatchId := mgobson.NewObjectId().Hex()
	previousPatchDoc := &patch.Patch{
		Id:            patch.NewId(prevPatchId),
		Activated:     true,
		Status:        evergreen.VersionFailed,
		Project:       s.project,
		CreateTime:    time.Now(),
		Author:        s.user,
		Version:       prevPatchId,
		Tasks:         []string{"t1", "t2"},
		BuildVariants: []string{"bv1"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"prevPatchTask"},
			},
		},
	}
	s.NoError((previousPatchDoc).Insert())

	earlierPatchDoc := &patch.Patch{
		Id:            patch.NewId(earlierPatchId),
		Activated:     true,
		Status:        evergreen.VersionFailed,
		Project:       s.project,
		CreateTime:    time.Now().Add(-time.Hour),
		Author:        s.user,
		Version:       earlierPatchId,
		Tasks:         []string{"t1", "t2", "t3", "t4"},
		BuildVariants: []string{"bv1"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"t1", "t2", "t3", "t4"},
			},
		},
	}
	s.NoError((earlierPatchDoc).Insert())

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:        s.user,
		Project:     s.project,
		BaseGitHash: s.hash,
		Description: s.desc,
		// --reuse flag with patch ID
		RepeatDefinition: true,
		RepeatPatchId:    earlierPatchId,
	})
	s.NoError(err)
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: s.user}

	project := model.Project{
		Identifier: s.project,
		BuildVariants: model.BuildVariants{
			{
				Name: "bv1",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name:    "t1",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t2",
						Variant: "bv1",
					},
					{
						Name:    "t3",
						Variant: "bv1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name:    "t4",
						Variant: "bv1",
					},
					{
						Name:    "prevPatchTask",
						Variant: "bv1",
					},
				},
			},
		},
		Tasks: []model.ProjectTask{
			{Name: "t1"},
			{Name: "t2"},
			{Name: "t3"},
			{Name: "t4"},
			{Name: "prevPatchTask"},
		},
	}

	currentPatchDoc := intent.NewPatch()

	s.NoError(err)

	// test --reuse with patch ID
	err = j.buildTasksAndVariants(currentPatchDoc, &project)
	s.NoError(err)
	sort.Strings(currentPatchDoc.Tasks)
	s.Equal([]string{"t1", "t2", "t3", "t4"}, currentPatchDoc.Tasks)
}

func (s *PatchIntentUnitsSuite) TestProcessMergeGroupIntent() {
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	headRef := "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
	orgName := "evergreen-ci"
	repoName := "commit-queue-sandbox"
	headSHA := "d2a90288ad96adca4a7d0122d8d4fd1deb24db11"
	baseSHA := "7a45fe6ea28f5969d885bc913e551b8b3c4f09d1"
	org := github.Organization{
		Login: &orgName,
	}
	repo := github.Repository{
		Name: &repoName,
	}
	mg := github.MergeGroup{
		HeadSHA: &headSHA,
		HeadRef: &headRef,
		BaseSHA: &baseSHA,
	}
	mge := github.MergeGroupEvent{
		MergeGroup: &mg,
		Org:        &org,
		Repo:       &repo,
	}
	intent, err := patch.NewGithubMergeIntent("id", "auto", &mge)

	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)

	patchDoc := intent.NewPatch()
	s.NoError(j.finishPatch(s.ctx, patchDoc))

	s.NoError(j.Error())
	s.False(j.HasErrors())

	dbPatch, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(dbPatch)
	s.True(patchDoc.Activated, "patch should be finalized")

	variants := []string{"ubuntu2004"}
	tasks := []string{"bynntask"}
	s.verifyPatchDoc(dbPatch, j.PatchID, baseSHA, false, variants, tasks)
	s.projectExists(j.PatchID.Hex())

	s.Zero(dbPatch.ProjectStorageMethod, "patch's project storage method should be unset after patch is finalized")
	s.verifyParserProjectDoc(dbPatch, 4)

	s.verifyVersionDoc(dbPatch, evergreen.GithubMergeRequester, evergreen.GithubMergeUser, baseSHA, 1)

	out := []event.Subscription{}
	s.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Query(bson.M{}), &out))
	s.Len(out, 2)
	for _, subscription := range out {
		s.Equal("github_merge", subscription.Subscriber.Type)
	}

	// Attempting to finalize again should result in no errors
	// The below is to clear what is set in a transaction.
	s.NoError(db.ClearCollections(model.VersionCollection, model.ProjectConfigCollection, manifest.Collection, build.Collection,
		task.Collection))
	patchDoc.Version = ""
	s.NoError(j.finishPatch(s.ctx, patchDoc))

	s.NoError(j.Error())
	s.False(j.HasErrors())
}

func (s *PatchIntentUnitsSuite) TestProcessGitHubIntentWithMergeBase() {
	pr := &github.PullRequest{
		Title: utility.ToStringPtr("Test title"),
		Base: &github.PullRequestBranch{
			SHA: github.String("ed42b5e51e81724c5258686a0b9d515a99696eac"),
			Repo: &github.Repository{
				FullName: utility.ToStringPtr("evergreen-ci/commit-queue-sandbox"),
			},
			Ref: utility.ToStringPtr("main"),
		},
		Head: &github.PullRequestBranch{
			SHA: github.String("ed42b5e51e81724c5258686a0b9d515a99696eac"),
			Repo: &github.Repository{
				FullName: utility.ToStringPtr("evergreen-ci/DEVPROD-123"),
			},
			Ref: utility.ToStringPtr("abc"),
		},
		User: &github.User{
			ID:    github.Int64(1),
			Login: utility.ToStringPtr("abc"),
		},
		Number:         github.Int(1),
		MergeCommitSHA: github.String("abcdef"),
	}
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	// SHA ed42b5e51e81724c5258686a0b9d515a99696eac is newer than the oldest allowed merge base and should be accepted
	intent, err := patch.NewGithubIntent("id", "auto", "", "", "ed42b5e51e81724c5258686a0b9d515a99696eac", pr)

	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())
	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	patchDoc := intent.NewPatch()
	s.NoError(j.finishPatch(s.ctx, patchDoc))
	s.Empty(j.gitHubError)

	// SHA 4aa79c5e7ef7af351764b843a2c05fab98c23881 is older than the oldest allowed merge base and should be rejected
	intent, err = patch.NewGithubIntent("another_id", "auto", "", "", "4aa79c5e7ef7af351764b843a2c05fab98c23881", pr)

	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())
	j = NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	patchDoc = intent.NewPatch()
	s.Error(j.finishPatch(s.ctx, patchDoc))
	s.NotEmpty(j.gitHubError)
	s.Equal(j.gitHubError, MergeBaseTooOld)
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntent() {
	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(s.ctx, flags))

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(s.ctx, "", s.githubPatchData)
	s.Require().NoError(err)
	s.Require().Len(summaries, 2)
	s.NotEmpty(patchContent)
	s.NotEqual("{", patchContent[0])

	s.Equal("cli/host.go", summaries[0].Name)
	s.Equal(2, summaries[0].Additions)
	s.Equal(6, summaries[0].Deletions)

	s.Equal("cli/keys.go", summaries[1].Name)
	s.Equal(1, summaries[1].Additions)
	s.Equal(3, summaries[1].Deletions)

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:         s.user,
		Project:      s.project,
		BaseGitHash:  s.hash,
		PatchContent: patchContent,
		Description:  s.desc,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
	})
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	s.NoError(j.finishPatch(s.ctx, patchDoc))

	s.NoError(j.Error())
	s.False(j.HasErrors())

	dbPatch, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(dbPatch)
	s.True(patchDoc.Activated, "patch should be finalized")

	variants := []string{"ubuntu1604", "ubuntu1604-arm64", "ubuntu1604-debug", "race-detector"}
	tasks := []string{"dist", "dist-test"}
	s.verifyPatchDoc(dbPatch, j.PatchID, s.hash, true, variants, tasks)
	s.projectExists(j.PatchID.Hex())

	s.Zero(dbPatch.ProjectStorageMethod, "patch's project storage method should be unset after patch is finalized")
	s.verifyParserProjectDoc(dbPatch, 8)

	s.verifyVersionDoc(dbPatch, evergreen.PatchVersionRequester, s.user, s.hash, 4)

	s.gridFSFileExists(dbPatch.Patches[0].PatchSet.PatchFileId)

	out := []event.Subscription{}
	s.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Query(bson.M{}), &out))
	s.Require().Empty(out)
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntentWithoutFinalizing() {
	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(s.ctx, flags))

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())

	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(s.ctx, "", s.githubPatchData)
	s.Require().NoError(err)
	s.Require().Len(summaries, 2)
	s.NotEmpty(patchContent)
	s.NotEqual("{", patchContent[0])

	s.Equal("cli/host.go", summaries[0].Name)
	s.Equal(2, summaries[0].Additions)
	s.Equal(6, summaries[0].Deletions)

	s.Equal("cli/keys.go", summaries[1].Name)
	s.Equal(1, summaries[1].Additions)
	s.Equal(3, summaries[1].Deletions)

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:         s.user,
		Project:      s.project,
		BaseGitHash:  s.hash,
		PatchContent: patchContent,
		Description:  s.desc,
		Variants:     s.variants,
		Tasks:        s.tasks,
	})
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := NewPatchIntentProcessor(s.env, mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	s.NoError(j.finishPatch(s.ctx, patchDoc))

	s.NoError(j.Error())
	s.False(j.HasErrors())

	dbPatch, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(dbPatch)
	s.False(patchDoc.Activated, "patch should not be finalized")

	variants := []string{"ubuntu1604", "ubuntu1604-arm64", "ubuntu1604-debug", "race-detector"}
	tasks := []string{"dist", "dist-test"}
	s.verifyPatchDoc(dbPatch, j.PatchID, s.hash, true, variants, tasks)
	s.projectExists(j.PatchID.Hex())

	s.Equal(evergreen.ProjectStorageMethodDB, dbPatch.ProjectStorageMethod, "unfinalized patch should have project storage method set")
	s.verifyParserProjectDoc(dbPatch, 8)

	dbVersion, err := model.VersionFindOne(model.VersionById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Zero(dbVersion, "should not create version for unfinalized patch")

	s.gridFSFileExists(dbPatch.Patches[0].PatchSet.PatchFileId)

	out := []event.Subscription{}
	s.NoError(db.FindAllQ(event.SubscriptionsCollection, db.Query(bson.M{}), &out))
	s.Require().Empty(out)
}

func (s *PatchIntentUnitsSuite) TestFindEvergreenUserForPR() {
	dbUser := user.DBUser{
		Id: "testuser",
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID:         1234,
				LastKnownAs: "somebody",
			},
		},
	}
	s.NoError(dbUser.Insert())

	u, err := findEvergreenUserForPR(1234)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal("testuser", u.Id)

	u, err = findEvergreenUserForPR(123)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal(evergreen.GithubPatchUser, u.Id)
}

func (s *PatchIntentUnitsSuite) TestFindEvergreenUserForGithubMergeGroup() {
	u, err := findEvergreenUserForGithubMergeGroup(123)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal(evergreen.GithubMergeUser, u.Id)
}

func (s *PatchIntentUnitsSuite) verifyPatchDoc(patchDoc *patch.Patch, expectedPatchID mgobson.ObjectId, hash string, verifyModules bool, variants []string, tasks []string) {
	s.Equal(evergreen.VersionCreated, patchDoc.Status)
	s.Equal(expectedPatchID, patchDoc.Id)
	if verifyModules {
		s.NotEmpty(patchDoc.Patches)
	}
	s.NotZero(patchDoc.CreateTime)
	s.Zero(patchDoc.GithubPatchData)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.NotEqual(0, patchDoc.PatchNumber)
	s.Equal(hash, patchDoc.Githash)

	if verifyModules {
		s.Len(patchDoc.Patches, 1)
		s.Empty(patchDoc.Patches[0].ModuleName)
		s.Equal(hash, patchDoc.Patches[0].Githash)
		s.Empty(patchDoc.Patches[0].PatchSet.Patch)
		s.NotEmpty(patchDoc.Patches[0].PatchSet.PatchFileId)
		s.Len(patchDoc.Patches[0].PatchSet.Summary, 2)
	}

	s.Len(patchDoc.BuildVariants, len(variants))
	for _, v := range variants {
		s.Contains(patchDoc.BuildVariants, v)
	}
	s.Len(patchDoc.Tasks, len(tasks))
	for _, t := range tasks {
		s.Contains(patchDoc.Tasks, t)
	}
	s.NotZero(patchDoc.CreateTime)
}

func (s *PatchIntentUnitsSuite) verifyParserProjectDoc(p *patch.Patch, variants int) {
	_, dbParserProject, err := model.FindAndTranslateProjectForPatch(s.ctx, s.env.Settings(), p)
	s.Require().NoError(err)
	s.Require().NotZero(dbParserProject)
	s.Len(dbParserProject.BuildVariants, variants)
}

func (s *PatchIntentUnitsSuite) projectExists(projectId string) {
	pp, err := model.ParserProjectFindOneByID(s.ctx, s.env.Settings(), evergreen.ProjectStorageMethodDB, projectId)
	s.NoError(err)
	s.NotNil(pp)
}

func (s *PatchIntentUnitsSuite) verifyVersionDoc(patchDoc *patch.Patch, expectedRequester, expectedUser, hash string, builds int) {
	versionDoc, err := model.VersionFindOne(model.VersionById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Require().NotNil(versionDoc)

	s.NotZero(versionDoc.CreateTime)
	s.Zero(versionDoc.StartTime)
	s.Zero(versionDoc.FinishTime)
	s.Equal(hash, versionDoc.Revision)
	s.Equal(patchDoc.Description, versionDoc.Message)
	s.Equal(expectedUser, versionDoc.Author)
	s.Len(versionDoc.BuildIds, builds)

	s.Require().Len(versionDoc.BuildVariants, builds)

	for i := 0; i < builds; i++ {
		s.True(versionDoc.BuildVariants[i].Activated)
		s.Zero(versionDoc.BuildVariants[i].ActivateAt)
		s.NotEmpty(versionDoc.BuildVariants[i].BuildId)
		s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[i].BuildId)
	}

	s.Equal(expectedRequester, versionDoc.Requester)
	s.Empty(versionDoc.Errors)
	s.Empty(versionDoc.Warnings)
	s.Empty(versionDoc.Owner)
	s.Empty(versionDoc.Repo)
	s.Equal(versionDoc.Id, patchDoc.Version)
}

func (s *PatchIntentUnitsSuite) gridFSFileExists(patchFileID string) {
	patchContents, err := patch.FetchPatchContents(patchFileID)
	s.Require().NoError(err)
	s.NotEmpty(patchContents)
}

func (s *PatchIntentUnitsSuite) TestRunInDegradedModeWithGithubIntent() {
	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(s.ctx, flags))

	intent, err := patch.NewGithubIntent("1", "", "", "", "", testutil.NewGithubPR(s.prNumber, s.repo, s.baseHash, s.headRepo, s.hash, "tychoish", "title1"))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(s.env, patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(s.ctx)
	s.Error(j.Error())
	s.Contains(j.Error().Error(), "not processing PR because GitHub PR testing is disabled")

	patchDoc, err := patch.FindOne(patch.ById(patchID))
	s.NoError(err)
	s.Nil(patchDoc)

	unprocessedIntents, err := patch.FindUnprocessedGithubIntents()
	s.Require().NoError(err)
	s.Require().Len(unprocessedIntents, 1)

	s.Equal(intent.ID(), unprocessedIntents[0].ID())
}

func (s *PatchIntentUnitsSuite) TestGithubPRTestFromUnknownUserDoesntCreateVersions() {
	flags := evergreen.ServiceFlags{
		GithubStatusAPIDisabled: true,
	}
	s.Require().NoError(evergreen.SetServiceFlags(s.ctx, flags))

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	intent, err := patch.NewGithubIntent("1", "", "", "", "", testutil.NewGithubPR(s.prNumber, "evergreen-ci/evergreen", s.baseHash, s.headRepo, "8a425038834326c212d65289e0c9e80e48d07e7e", "octocat", "title1"))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(s.env, patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(s.ctx)
	s.Error(j.Error())
	filter := patch.ById(patchID)
	patchDoc, err := patch.FindOne(filter)
	s.NoError(err)
	s.Require().NotNil(patchDoc)
	s.Empty(patchDoc.Version)

	versionDoc, err := model.VersionFindOne(model.VersionById(patchID.Hex()))
	s.NoError(err)
	s.Nil(versionDoc)

	unprocessedIntents, err := patch.FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Require().Empty(unprocessedIntents)
}

func (s *PatchIntentUnitsSuite) TestGetModulePatch() {
	s.Require().NoError(db.ClearGridCollections(patch.GridFSPrefix))
	patchString := `diff --git a/test.txt b/test.txt
index ca20f6c..224168e 100644
--- a/test.txt
+++ b/test.txt
@@ -15,1 +15,1 @@ func myFunc() {
-  old line",
+  new line",

`
	s.Require().NoError(db.WriteGridFile(patch.GridFSPrefix, "testPatch", strings.NewReader(patchString)))

	modulePatch := patch.ModulePatch{}
	modulePatch.PatchSet.PatchFileId = "testPatch"
	modulePatch, err := getModulePatch(modulePatch)
	s.NotEmpty(modulePatch.PatchSet.Summary)
	s.NoError(err)
}

func (s *PatchIntentUnitsSuite) TestCliBackport() {
	sourcePatch := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: s.project,
		Githash: s.hash,
		Alias:   evergreen.CommitQueueAlias,
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
		},
		Patches: []patch.ModulePatch{
			{
				Githash: "revision",
				PatchSet: patch.PatchSet{
					Patch: "something",
					Summary: []thirdparty.Summary{
						{Name: "asdf", Additions: 4, Deletions: 80},
						{Name: "random.txt", Additions: 6, Deletions: 0},
					},
				},
			},
		},
	}
	s.NoError(sourcePatch.Insert())
	params := patch.CLIIntentParams{
		User:        s.user,
		Project:     s.project,
		BaseGitHash: s.hash,
		BackportOf: patch.BackportInfo{
			PatchID: sourcePatch.Id.Hex(),
		},
	}

	intent, err := patch.NewCliIntent(params)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), s.T().Name())
	id := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(s.env, id, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(s.ctx)
	s.NoError(j.Error())

	backportPatch, err := patch.FindOneId(id.Hex())
	s.NoError(err)
	s.Equal(sourcePatch.Id.Hex(), backportPatch.BackportOf.PatchID)
	s.Len(backportPatch.Patches, 1)
	s.Equal(sourcePatch.Patches[0].PatchSet.Patch, backportPatch.Patches[0].PatchSet.Patch)
}

func (s *PatchIntentUnitsSuite) TestProcessTriggerAliases() {
	latestVersion := model.Version{
		Id:         "childProj-some-version",
		Identifier: "childProj",
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	s.Require().NoError(latestVersion.Insert())

	latestVersionParserProject := &model.ParserProject{}
	s.Require().NoError(util.UnmarshalYAMLWithFallback([]byte(`
buildvariants:
- name: my-build-variant
  display_name: my-build-variant
  run_on:
    - some-distro
  tasks:
    - my-task
tasks:
- name: my-task`), latestVersionParserProject))
	latestVersionParserProject.Id = latestVersion.Id
	s.Require().NoError(latestVersionParserProject.Insert())

	childPatchAlias := model.ProjectAlias{
		ProjectID: "childProj",
		Alias:     "childProj-patch-alias",
		Task:      "my-task",
		Variant:   "my-build-variant",
	}
	s.Require().NoError(childPatchAlias.Upsert())

	p := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: s.project,
		Author:  evergreen.GithubPatchUser,
		Githash: s.hash,
	}
	s.NoError(p.Insert())
	pp := &model.ParserProject{
		Id: p.Id.Hex(),
	}
	s.NoError(pp.Insert())

	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	s.NoError(u.Insert())

	projectRef, err := model.FindBranchProjectRef(s.project)
	s.Require().NotNil(projectRef)
	s.NoError(err)

	s.Len(p.Triggers.ChildPatches, 0)
	s.NoError(ProcessTriggerAliases(s.ctx, p, projectRef, s.env, []string{"patch-alias"}))

	dbPatch, err := patch.FindOneId(p.Id.Hex())
	s.NoError(err)
	s.Require().NotZero(dbPatch)
	s.Equal(p.Triggers.ChildPatches, dbPatch.Triggers.ChildPatches)

	s.Require().Len(dbPatch.Triggers.ChildPatches, 1)
	dbChildPatch, err := patch.FindOneId(dbPatch.Triggers.ChildPatches[0])
	s.NoError(err)
	s.Require().NotZero(dbChildPatch)
	s.Require().Len(dbChildPatch.VariantsTasks, 1, "child patch with valid trigger alias in the child project must have the expected variants and tasks")
	s.Equal("my-build-variant", dbChildPatch.VariantsTasks[0].Variant)
	s.Require().Len(dbChildPatch.VariantsTasks[0].Tasks, 1, "child patch with valid trigger alias in the child project must have the expected variants and tasks")
	s.Equal("my-task", dbChildPatch.VariantsTasks[0].Tasks[0])
}

func (s *PatchIntentUnitsSuite) TestTriggerAliasWithDownstreamRevision() {
	specificRevision := model.Version{
		Id:         "childProj-some-version",
		Identifier: "childProj",
		Revision:   "abc",
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	s.Require().NoError(specificRevision.Insert())

	specificVersionParserProject := &model.ParserProject{}
	s.Require().NoError(util.UnmarshalYAMLWithFallback([]byte(`
buildvariants:
- name: my-build-variant
  display_name: my-build-variant
  run_on:
    - some-distro
  tasks:
    - my-task
tasks:
- name: my-task`), specificVersionParserProject))
	specificVersionParserProject.Id = specificRevision.Id
	s.Require().NoError(specificVersionParserProject.Insert())

	childPatchAlias := model.ProjectAlias{
		ProjectID: "childProj",
		Alias:     "childProj-patch-alias",
		Task:      "my-task",
		Variant:   "my-build-variant",
	}
	s.Require().NoError(childPatchAlias.Upsert())

	p := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: s.project,
		Author:  evergreen.GithubPatchUser,
		Githash: s.hash,
	}
	s.NoError(p.Insert())

	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	s.NoError(u.Insert())

	projectRef, err := model.FindBranchProjectRef(s.project)
	s.Require().NotNil(projectRef)
	s.NoError(err)
	s.Require().Len(projectRef.PatchTriggerAliases, 1)
	projectRef.PatchTriggerAliases[0].DownstreamRevision = "abc"

	s.NoError(ProcessTriggerAliases(s.ctx, p, projectRef, s.env, []string{"patch-alias"}))

	dbPatch, err := patch.FindOneId(p.Id.Hex())
	s.NoError(err)
	s.Require().NotZero(dbPatch)
	s.Equal(p.Triggers.ChildPatches, dbPatch.Triggers.ChildPatches)

	s.Require().Len(dbPatch.Triggers.ChildPatches, 1)
	dbChildPatch, err := patch.FindOneId(dbPatch.Triggers.ChildPatches[0])
	s.NoError(err)
	s.Require().NotZero(dbChildPatch)
	s.Require().Len(dbChildPatch.VariantsTasks, 1, "child patch with valid trigger alias in the child project must have the expected variants and tasks")
	s.Equal("my-build-variant", dbChildPatch.VariantsTasks[0].Variant)
	s.Require().Len(dbChildPatch.VariantsTasks[0].Tasks, 1, "child patch with valid trigger alias in the child project must have the expected variants and tasks")
	s.Equal("my-task", dbChildPatch.VariantsTasks[0].Tasks[0])
}

func (s *PatchIntentUnitsSuite) TestProcessTriggerAliasesWithAliasThatDoesNotMatchAnyVariantTasks() {
	p := &patch.Patch{
		Id:      mgobson.NewObjectId(),
		Project: s.project,
		Author:  evergreen.GithubPatchUser,
		Githash: s.hash,
	}
	s.NoError(p.Insert())
	pp := &model.ParserProject{
		Id: p.Id.Hex(),
	}
	s.NoError(pp.Insert())
	latestVersion := model.Version{
		Id:         "childProj-some-version",
		Identifier: "childProj",
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	s.Require().NoError(latestVersion.Insert())

	latestVersionParserProject := &model.ParserProject{}
	s.Require().NoError(util.UnmarshalYAMLWithFallback([]byte(`
buildvariants:
- name: my-build-variant
  display_name: my-build-variant
  run_on:
    - some-distro
  tasks:
    - my-task
tasks:
- name: my-task`), latestVersionParserProject))
	latestVersionParserProject.Id = latestVersion.Id
	s.Require().NoError(latestVersionParserProject.Insert())

	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	s.NoError(u.Insert())

	projectRef, err := model.FindBranchProjectRef(s.project)
	s.Require().NotNil(projectRef)
	s.NoError(err)

	s.Len(p.Triggers.ChildPatches, 0)
	s.Error(ProcessTriggerAliases(s.ctx, p, projectRef, s.env, []string{"patch-alias"}), "should error if no tasks/variants match")
	s.Len(p.Triggers.ChildPatches, 1)

	dbPatch, err := patch.FindOneId(p.Id.Hex())
	s.NoError(err)
	s.Equal(p.Triggers.ChildPatches, dbPatch.Triggers.ChildPatches)
}
