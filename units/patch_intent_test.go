package units

import (
	"context"
	"io/ioutil"
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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type PatchIntentUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc

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

	suite.Run(t, new(PatchIntentUnitsSuite))
}

func (s *PatchIntentUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx))

	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), "TestPatchIntentUnitsSuite")
	s.NotNil(s.env.Settings())

	s.NoError(db.ClearCollections(evergreen.ConfigCollection, task.Collection, model.ProjectVarsCollection, model.VersionCollection, user.Collection, model.ProjectRefCollection, patch.Collection, patch.IntentCollection, event.SubscriptionsCollection, distro.Collection))
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	s.NoError((&model.ProjectRef{
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Id:               "mci",
		Enabled:          utility.TruePtr(),
		PatchingDisabled: utility.FalsePtr(),
		Branch:           "main",
		RemotePath:       "self-tests.yml",
		PRTestingEnabled: utility.TruePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		PatchTriggerAliases: []patch.PatchTriggerDefinition{
			{Alias: "patch-alias", ChildProject: "childProj"},
		},
	}).Insert())

	s.NoError((&model.ProjectRef{
		Id:         "childProj",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "main",
		Enabled:    utility.TruePtr(),
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

	s.NoError((&distro.Distro{Id: "ubuntu1604-test"}).Insert())
	s.NoError((&distro.Distro{Id: "ubuntu1604-build"}).Insert())
	s.NoError((&distro.Distro{Id: "archlinux-test"}).Insert())
	s.NoError((&distro.Distro{Id: "archlinux-build"}).Insert())
	s.NoError((&distro.Distro{Id: "windows-64-vs2015-small"}).Insert())
	s.NoError((&distro.Distro{Id: "rhel71-power8-test"}).Insert())
	s.NoError((&distro.Distro{Id: "rhel72-zseries-test"}).Insert())
	s.NoError((&distro.Distro{Id: "ubuntu1604-arm64-small"}).Insert())
	s.NoError((&distro.Distro{Id: "rhel62-test"}).Insert())
	s.NoError((&distro.Distro{Id: "rhel70-small"}).Insert())
	s.NoError((&distro.Distro{Id: "rhel62-small"}).Insert())
	s.NoError((&distro.Distro{Id: "linux-64-amzn-test"}).Insert())
	s.NoError((&distro.Distro{Id: "debian81-test"}).Insert())
	s.NoError((&distro.Distro{Id: "debian71-test"}).Insert())
	s.NoError((&distro.Distro{Id: "ubuntu1404-test"}).Insert())
	s.NoError((&distro.Distro{Id: "macos-1012"}).Insert())

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
func (s *PatchIntentUnitsSuite) TearDownTest() {
	s.cancel()
}

func (s *PatchIntentUnitsSuite) makeJobAndPatch(intent patch.Intent, patchedParserProject string) *patchIntentProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	if patchedParserProject != "" {
		patchDoc.PatchedParserProject = patchedParserProject
		patchDoc.Patches = append(patchDoc.Patches, patch.ModulePatch{ModuleName: "sandbox"})
	}
	s.NoError(j.finishPatch(ctx, patchDoc, githubOauthToken))
	s.NoError(j.Error())
	s.False(j.HasErrors())

	return j
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithNoTasksAndVariants() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
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

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(ctx, patchDoc, githubOauthToken)
	s.Require().Error(err)
	s.Equal("patch has no build variants or tasks", err.Error())
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithBadAlias() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
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

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(ctx, patchDoc, githubOauthToken)
	s.Require().Error(err)
	s.Equal("alias 'typo' could not be found on project 'mci'", err.Error())
}

func (s *PatchIntentUnitsSuite) TestCantFinishCommitQueuePatchWithNoTasksAndVariants() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
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

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(ctx, patchDoc, githubOauthToken)
	s.Require().Error(err)
	s.Equal("patch has no build variants or tasks", err.Error())
}

func (s *PatchIntentUnitsSuite) TestSetToPreviousPatchDefinition() {
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
		BuildVariant:   "bv_only_dt",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et1"},
		Status:         evergreen.TaskFailed,
		Activated:      true,
	}
	et1 := task.Task{
		Id:            "et1",
		DisplayName:   "et1",
		Version:       "v1",
		BuildVariant:  "bv_only_dt",
		DisplayTaskId: utility.ToStringPtr("dt1"),
		Status:        evergreen.TaskFailed,
		Activated:     true,
	}
	dt2 := task.Task{
		Id:             "dt2",
		DisplayName:    "dt2",
		Version:        "v1",
		BuildVariant:   "bv_different_dt",
		DisplayOnly:    true,
		ExecutionTasks: []string{"et2"},
		Status:         evergreen.TaskSucceeded,
		Activated:      true,
	}
	et2 := task.Task{
		Id:            "et2",
		DisplayName:   "et2",
		Version:       "v1",
		BuildVariant:  "bv_different_dt",
		DisplayTaskId: utility.ToStringPtr("dt2"),
		Status:        evergreen.TaskSucceeded,
		Activated:     false,
	}

	s.NoError(t1.Insert())
	s.NoError(t2.Insert())
	s.NoError(tgt1.Insert())
	s.NoError(tgt2.Insert())
	s.NoError(tgt3.Insert())
	s.NoError(tgt4.Insert())
	s.NoError(dt1.Insert())
	s.NoError(et1.Insert())
	s.NoError(dt2.Insert())
	s.NoError(et2.Insert())
	s.NoError(notActivated.Insert())
	patchId := "aabbccddeeff001122334455"
	previousPatchDoc := &patch.Patch{
		Id:         patch.NewId(patchId),
		Activated:  true,
		Status:     evergreen.PatchFailed,
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
				Variant:      "bv_only_dt",
				DisplayTasks: []patch.DisplayTask{{Name: "dt1", ExecTasks: []string{"et1"}}},
				Tasks:        []string{"et1"},
			},
			{
				Variant:      "bv_different_dt",
				DisplayTasks: []patch.DisplayTask{{Name: "dt2", ExecTasks: []string{"et2"}}},
				Tasks:        []string{"et2"},
			},
		},
		Tasks:              []string{"t1", "t2", "tgt1", "tgt2", "tgt3", "tgt4", "dt1", "et1", "dt2", "et2", "not_activated"},
		BuildVariants:      []string{"bv_only_dt", "bv_different_dt"},
		RegexBuildVariants: []string{"bv_$"},
		RegexTasks:         []string{"1$"},
	}
	s.NoError((previousPatchDoc).Insert())

	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:             "me",
		Project:          s.project,
		BaseGitHash:      s.hash,
		Description:      s.desc,
		RepeatDefinition: true,
	})
	s.NoError(err)
	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: "me"}
	project := model.Project{Identifier: s.project, BuildVariants: model.BuildVariants{
		{
			Name: "bv1",
			Tasks: []model.BuildVariantTaskUnit{
				{Name: "t1"},
				{Name: "tg", IsGroup: true},
				{Name: "tg2", IsGroup: true},
			},
		},
		{
			Name:         "bv_only_dt",
			DisplayTasks: []patch.DisplayTask{{Name: "dt1", ExecTasks: []string{"et1"}}},
			Tasks: []model.BuildVariantTaskUnit{
				{Name: "et1"},
			},
		},
		{
			Name:         "bv_different_dt",
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
	currentPatchDoc := intent.NewPatch()
	s.NoError(err)
	previousPatchStatus, err := j.setToPreviousPatchDefinition(currentPatchDoc, &project, false)
	s.NoError(err)
	s.Equal(previousPatchStatus, "failed")

	s.Equal(currentPatchDoc.BuildVariants, previousPatchDoc.BuildVariants)
	s.Equal(currentPatchDoc.Tasks, []string{"t1", "t2", "tgt1", "tgt2", "tgt4", "et1"})

	previousPatchStatus, err = j.setToPreviousPatchDefinition(currentPatchDoc, &project, true)
	s.NoError(err)
	s.Equal(previousPatchStatus, "failed")
	s.Equal(currentPatchDoc.BuildVariants, previousPatchDoc.BuildVariants)
	s.Equal(currentPatchDoc.Tasks, []string{"t1", "tgt1", "tgt2", "tgt4", "et1"})
}

func (s *PatchIntentUnitsSuite) TestBuildTasksandVariantsWithRepeatFailed() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(patch.Collection))

	patchId := "aaaaaaaaaaff001122334455"
	tasks := []task.Task{
		{
			Id:           "t1",
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
	}

	for _, t := range tasks {
		s.NoError(t.Insert())
	}

	previousPatchDoc := &patch.Patch{
		Id:            patch.NewId(patchId),
		Activated:     true,
		Status:        evergreen.PatchFailed,
		Project:       s.project,
		CreateTime:    time.Now(),
		Author:        s.user,
		Version:       patchId,
		Tasks:         []string{"t1", "t2"},
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
	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: s.user}

	project := model.Project{
		Identifier: s.project,
		BuildVariants: model.BuildVariants{
			{
				Name: "bv1",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name: "t1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskSucceeded},
						},
					},
					{
						Name: "t2",
					},
					{
						Name: "t3",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name: "t4",
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
	currentPatchDoc.Tasks = []string{"t1", "t2"}
	currentPatchDoc.BuildVariants = []string{"bv1"}

	s.NoError(err)

	err = j.buildTasksandVariants(currentPatchDoc, &project)
	s.NoError(err)
	sort.Strings(currentPatchDoc.Tasks)
	s.Equal([]string{"t1", "t3", "t4"}, currentPatchDoc.Tasks)
}

func (s *PatchIntentUnitsSuite) TestBuildTasksandVariantsWithReuse() {
	s.Require().NoError(db.ClearCollections(task.Collection))
	s.Require().NoError(db.ClearCollections(patch.Collection))

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
	}

	for _, t := range tasks {
		s.NoError(t.Insert())
	}

	previousPatchDoc := &patch.Patch{
		Id:            patch.NewId(patchId),
		Activated:     true,
		Status:        evergreen.PatchFailed,
		Project:       s.project,
		CreateTime:    time.Now(),
		Author:        s.user,
		Version:       patchId,
		Tasks:         []string{"t1", "t2"},
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
		// --reuse flag
		RepeatDefinition: true,
	})
	s.NoError(err)
	j := NewPatchIntentProcessor(mgobson.NewObjectId(), intent).(*patchIntentProcessor)
	j.user = &user.DBUser{Id: s.user}

	project := model.Project{
		Identifier: s.project,
		BuildVariants: model.BuildVariants{
			{
				Name: "bv1",
				Tasks: []model.BuildVariantTaskUnit{
					{
						Name: "t1",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t3", Status: evergreen.TaskFailed},
						},
					},
					{
						Name: "t2",
					},
					{
						Name: "t3",
						DependsOn: []model.TaskUnitDependency{
							{Name: "t4", Status: evergreen.TaskFailed},
						},
					},
					{
						Name: "t4",
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
	currentPatchDoc.Tasks = []string{"t1", "t2"}
	currentPatchDoc.BuildVariants = []string{"bv1"}

	s.NoError(err)

	// test --reuse
	err = j.buildTasksandVariants(currentPatchDoc, &project)
	s.NoError(err)
	sort.Strings(currentPatchDoc.Tasks)
	s.Equal([]string{"t1", "t2", "t3", "t4"}, currentPatchDoc.Tasks)
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntent() {
	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(flags))

	patchContent, summaries, err := thirdparty.GetGithubPullRequestDiff(context.Background(), githubOauthToken, s.githubPatchData)
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
	j := s.makeJobAndPatch(intent, "")

	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)

	s.NotZero(patchDoc.CreateTime)
	s.Zero(patchDoc.GithubPatchData)

	s.verifyVersionDoc(patchDoc, evergreen.PatchVersionRequester)

	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)

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

func (s *PatchIntentUnitsSuite) verifyPatchDoc(patchDoc *patch.Patch, expectedPatchID mgobson.ObjectId) {
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.Equal(expectedPatchID, patchDoc.Id)
	s.NotEmpty(patchDoc.Patches)
	s.True(patchDoc.Activated)
	s.NotEmpty(patchDoc.PatchedParserProject)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.NotEqual(0, patchDoc.PatchNumber)
	s.Equal(s.hash, patchDoc.Githash)

	s.Len(patchDoc.Patches, 1)
	s.Empty(patchDoc.Patches[0].ModuleName)
	s.Equal(s.hash, patchDoc.Patches[0].Githash)
	s.Empty(patchDoc.Patches[0].PatchSet.Patch)
	s.NotEmpty(patchDoc.Patches[0].PatchSet.PatchFileId)
	s.Len(patchDoc.Patches[0].PatchSet.Summary, 2)

	s.Len(patchDoc.BuildVariants, 4)
	s.Contains(patchDoc.BuildVariants, "ubuntu1604")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-arm64")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-debug")
	s.Contains(patchDoc.BuildVariants, "race-detector")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "dist")
	s.Contains(patchDoc.Tasks, "dist-test")
	s.NotZero(patchDoc.CreateTime)
}

func (s *PatchIntentUnitsSuite) verifyVersionDoc(patchDoc *patch.Patch, expectedRequester string) {
	versionDoc, err := model.VersionFindOne(model.VersionById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Require().NotNil(versionDoc)

	s.NotZero(versionDoc.CreateTime)
	s.Zero(versionDoc.StartTime)
	s.Zero(versionDoc.FinishTime)
	s.Equal(s.hash, versionDoc.Revision)
	s.Equal(patchDoc.Description, versionDoc.Message)
	s.Equal(s.user, versionDoc.Author)
	s.Len(versionDoc.BuildIds, 4)

	s.Require().Len(versionDoc.BuildVariants, 4)
	s.True(versionDoc.BuildVariants[0].Activated)
	s.Zero(versionDoc.BuildVariants[0].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[0].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[0].BuildId)

	s.True(versionDoc.BuildVariants[1].Activated)
	s.Zero(versionDoc.BuildVariants[1].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[1].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[1].BuildId)

	s.True(versionDoc.BuildVariants[2].Activated)
	s.Zero(versionDoc.BuildVariants[2].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[2].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[2].BuildId)

	s.True(versionDoc.BuildVariants[3].Activated)
	s.Zero(versionDoc.BuildVariants[3].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[3].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[3].BuildId)

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
	s.NoError(evergreen.SetServiceFlags(flags))

	intent, err := patch.NewGithubIntent("1", "", "", testutil.NewGithubPR(s.prNumber, s.repo, s.baseHash, s.headRepo, s.hash, "tychoish", "title1"))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(context.Background())
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
	s.Require().NoError(evergreen.SetServiceFlags(flags))

	intent, err := patch.NewGithubIntent("1", "", "", testutil.NewGithubPR(s.prNumber, "evergreen-ci/evergreen", s.baseHash, s.headRepo, "8a425038834326c212d65289e0c9e80e48d07e7e", "octocat", "title1"))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(context.Background())
	s.Error(j.Error())
	filter := patch.ById(patchID)
	patchDoc, err := patch.FindOne(filter)
	s.NoError(err)
	if s.NotNil(patchDoc) {
		s.Empty(patchDoc.Version)
	}

	versionDoc, err := model.VersionFindOne(model.VersionById(patchID.Hex()))
	s.NoError(err)
	s.Nil(versionDoc)

	unprocessedIntents, err := patch.FindUnprocessedGithubIntents()
	s.Require().NoError(err)
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

	id := mgobson.NewObjectId()
	j, ok := NewPatchIntentProcessor(id, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(context.Background())
	s.NoError(j.Error())

	backportPatch, err := patch.FindOneId(id.Hex())
	s.NoError(err)
	s.Equal(sourcePatch.Id.Hex(), backportPatch.BackportOf.PatchID)
	s.Len(backportPatch.Patches, 1)
	s.Equal(sourcePatch.Patches[0].PatchSet.Patch, backportPatch.Patches[0].PatchSet.Patch)
}

const PatchId = "58d156352cfeb61064cf08b3"

func (s *PatchIntentUnitsSuite) TestProcessTriggerAliases() {
	evergreen.GetEnvironment().Settings().Credentials = s.env.Settings().Credentials

	p := &patch.Patch{
		Id:      patch.NewId(PatchId),
		Project: s.project,
		Author:  evergreen.GithubPatchUser,
		Githash: s.hash,
	}
	s.NoError(p.Insert())

	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	s.NoError(u.Insert())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	projectRef, err := model.FindBranchProjectRef(s.project)
	s.NotNil(projectRef)
	s.NoError(err)

	s.Len(p.Triggers.ChildPatches, 0)
	s.NoError(ProcessTriggerAliases(ctx, p, projectRef, s.env, []string{"patch-alias"}))
	s.Len(p.Triggers.ChildPatches, 1)

	dbPatch, err := patch.FindOneId(p.Id.Hex())
	s.NoError(err)
	s.Equal(p.Triggers.ChildPatches, dbPatch.Triggers.ChildPatches)
}
