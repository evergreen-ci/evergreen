package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/yaml.v3"
)

func init() {
	testutil.Setup()
}

func TestFindProject(t *testing.T) {
	assert.NoError(t, db.Clear(ProjectRefCollection))
	projRef := &ProjectRef{
		Id: "project_test",
	}
	assert.NoError(t, projRef.Insert())
	Convey("When finding a project", t, func() {
		Convey("an error should be thrown if the project ref's identifier is nil", func() {
			projRef := &ProjectRef{
				Id: "",
			}
			version, project, pp, err := FindLatestVersionWithValidProject(projRef.Id, false)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
			So(pp, ShouldBeNil)
			So(version, ShouldBeNil)
		})

		Convey("if the project file exists and is valid, the project spec within"+
			"should be unmarshalled and returned", func() {
			So(db.ClearCollections(VersionCollection, ParserProjectCollection), ShouldBeNil)
			v := &Version{
				Id:         "my_version",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			pp := ParserProject{
				Id: "my_version",
			}
			p := &ProjectRef{
				Id:     "project_test",
				Owner:  "fakeowner",
				Repo:   "fakerepo",
				Branch: "fakebranch",
			}
			require.NoError(t, pp.Insert())
			require.NoError(t, v.Insert(), "failed to insert test version: %v", v)
			_, _, _, err := FindLatestVersionWithValidProject(p.Id, false)
			So(err, ShouldBeNil)

		})
		Convey("if the first version is somehow malformed, return an earlier one", func() {
			So(db.ClearCollections(VersionCollection, ParserProjectCollection), ShouldBeNil)
			badVersion := &Version{
				Id:                  "bad_version",
				Owner:               "fakeowner",
				Repo:                "fakerepo",
				Branch:              "fakebranch",
				Identifier:          "project_test",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 10,
				Errors:              []string{"this is a bad version"},
			}
			goodVersion := &Version{
				Id:                  "good_version",
				Owner:               "fakeowner",
				Repo:                "fakerepo",
				Branch:              "fakebranch",
				Identifier:          "project_test",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 8,
			}
			pp := &ParserProject{}
			err := util.UnmarshalYAMLWithFallback([]byte("owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch"), &pp)
			So(err, ShouldBeNil)
			pp.Id = "good_version"
			So(badVersion.Insert(), ShouldBeNil)
			So(goodVersion.Insert(), ShouldBeNil)
			So(pp.Insert(), ShouldBeNil)
			v, p, pp, err := FindLatestVersionWithValidProject("project_test", false)
			So(err, ShouldBeNil)
			So(pp, ShouldNotBeNil)
			So(pp.Id, ShouldEqual, "good_version")
			So(p, ShouldNotBeNil)
			So(v.Id, ShouldEqual, "good_version")
		})
		Convey("if a pre generation parser project exists, it should be pulled", func() {
			So(db.ClearCollections(VersionCollection, ParserProjectCollection), ShouldBeNil)
			goodVersion := &Version{
				Id:                  "good_version",
				Owner:               "fakeowner",
				Repo:                "fakerepo",
				Branch:              "fakebranch",
				Identifier:          "project_test",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 8,
			}
			pp := &ParserProject{}
			err := util.UnmarshalYAMLWithFallback([]byte("owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch"), &pp)
			So(err, ShouldBeNil)
			pp.Id = "good_version"
			So(goodVersion.Insert(), ShouldBeNil)
			So(pp.Insert(), ShouldBeNil)
			pp.Id = "pre_generation_good_version"
			So(pp.Insert(), ShouldBeNil)
			v, p, pp, err := FindLatestVersionWithValidProject("project_test", true)
			So(err, ShouldBeNil)
			So(pp, ShouldNotBeNil)
			So(pp.Id, ShouldEqual, "pre_generation_good_version")
			So(p, ShouldNotBeNil)
			So(v.Id, ShouldEqual, "good_version")
		})
		Convey("error if no version exists", func() {
			So(db.ClearCollections(VersionCollection, ParserProjectCollection), ShouldBeNil)
			_, _, _, err := FindLatestVersionWithValidProject("project_test", false)
			So(err, ShouldNotBeNil)
		})
	})

}

func TestFindTaskGroupForTask(t *testing.T) {
	parserProject := &ParserProject{
		Tasks: []parserTask{
			{Name: "t1"},
			{Name: "t2"},
			{Name: "tg1t1"},
			{Name: "tg1t2"},
			{Name: "tg1t3"},
			{Name: "tg1t4"},
			{Name: "tg2t1"},
			{Name: "tg2t2"},
			{Name: "tg3t1"},
			{Name: "tg3t2"},
			{Name: "tg4t1"},
			{Name: "tg4t2"},
		},
		BuildVariants: []parserBV{
			{Name: "v1", Tasks: []parserBVTaskUnit{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "tg1t1"},
				{Name: "tg1t2"},
				{Name: "tg1t3"},
				{Name: "tg1t4"},
				{Name: "tg3"},
			}},
			{Name: "v2", Tasks: []parserBVTaskUnit{
				{Name: "t1"},
				{Name: "tg2t1"},
				{Name: "tg2t2"},
			}},
		},
		TaskGroups: []parserTaskGroup{
			{Name: "tg1", Tasks: []string{"tg1t1", "tg1t2", "tg1t3", "tg1t4"}},
			{Name: "tg2", Tasks: []string{"tg2t1", "tg2t2"}},
			{Name: "tg3", Tasks: []string{"tg3t1", "tg3t2"}},
		},
	}
	p, err := TranslateProject(parserProject)
	require.NoError(t, err)
	assert.NotNil(t, p)

	t.Run("NotFoundTaskGroupWhenDefinedIndividually", func(t *testing.T) {
		tg1Tasks := []string{"tg1t1", "tg1t2", "tg1t3", "tg1t4"}
		for _, task := range tg1Tasks {
			tg := p.FindTaskGroupForTask("v1", task)
			require.Nil(t, tg, "finding task group for task %s", task)
		}

		tg2Tasks := []string{"tg2t1", "tg2t2"}
		for _, task := range tg2Tasks {
			tg := p.FindTaskGroupForTask("v2", task)
			require.Nil(t, tg, "finding task group for task %s", task)
		}
	})

	t.Run("FindsTaskGroupWhenDefinedAsATaskGroup", func(t *testing.T) {
		tg3Tasks := []string{"tg3t1", "tg3t2"}
		for _, task := range tg3Tasks {
			tg := p.FindTaskGroupForTask("v1", task)
			require.NotNil(t, tg, "finding task group for task %s", task)
			assert.Equal(t, "tg3", tg.Name)
		}
	})

	t.Run("ReturnsNilForNonTaskGroupTask", func(t *testing.T) {
		tg := p.FindTaskGroupForTask("v1", "t1")
		assert.Nil(t, tg)
	})
}

func TestGetVariantMappings(t *testing.T) {

	Convey("With a project", t, func() {

		Convey("getting variant mappings should return a map of the build"+
			" variant names to their display names", func() {

			project := &Project{
				BuildVariants: []BuildVariant{
					{
						Name:        "bv1",
						DisplayName: "bv1",
					},
					{
						Name:        "bv2",
						DisplayName: "dsp2",
					},
					{
						Name:        "blecch",
						DisplayName: "blecchdisplay",
					},
				},
			}

			mappings := project.GetVariantMappings()
			So(len(mappings), ShouldEqual, 3)
			So(mappings["bv1"], ShouldEqual, "bv1")
			So(mappings["bv2"], ShouldEqual, "dsp2")
			So(mappings["blecch"], ShouldEqual, "blecchdisplay")

		})

	})

}

func TestPopulateBVT(t *testing.T) {

	Convey("With a test Project and BuildVariantTaskUnit", t, func() {

		project := &Project{
			Tasks: []ProjectTask{
				{
					Name:            "task1",
					ExecTimeoutSecs: 500,
					Stepback:        utility.FalsePtr(),
					DependsOn:       []TaskUnitDependency{{Name: "other"}},
					Priority:        1000,
					Patchable:       utility.FalsePtr(),
				},
			},
			BuildVariants: []BuildVariant{
				{
					Name:  "test",
					Tasks: []BuildVariantTaskUnit{{Name: "task1", Variant: "test", Priority: 5}},
				},
			},
		}

		Convey("updating a BuildVariantTaskUnit with unset fields", func() {
			bvt := project.BuildVariants[0].Tasks[0]
			projectTask := project.FindProjectTask("task1")
			So(projectTask, ShouldNotBeNil)
			So(projectTask.Name, ShouldEqual, "task1")
			bvt.Populate(*projectTask, project.BuildVariants[0])

			Convey("should inherit the unset fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.Stepback, ShouldNotBeNil)
				So(bvt.Patchable, ShouldNotBeNil)
				So(len(bvt.DependsOn), ShouldEqual, 1)

				Convey("but not set fields", func() { So(bvt.Priority, ShouldEqual, 5) })
			})
		})

		Convey("updating a BuildVariantTaskUnit with set fields", func() {
			bvt := BuildVariantTaskUnit{
				Name:      "task1",
				Variant:   "bv",
				Stepback:  utility.TruePtr(),
				DependsOn: []TaskUnitDependency{{Name: "task2"}, {Name: "task3"}},
			}
			projectTask := project.FindProjectTask("task1")
			So(projectTask, ShouldNotBeNil)
			So(projectTask.Name, ShouldEqual, "task1")
			bvt.Populate(*projectTask, project.BuildVariants[0])

			Convey("should not inherit set fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.Stepback, ShouldNotBeNil)
				So(*bvt.Stepback, ShouldBeTrue)
				So(len(bvt.DependsOn), ShouldEqual, 2)

				Convey("but unset fields should", func() { So(bvt.Priority, ShouldEqual, 1000) })
			})
		})
	})
}

func TestIgnoresAllFiles(t *testing.T) {
	Convey("With test Project.Ignore setups and a list of.py, .yml, and .md files", t, func() {
		files := []string{
			"src/cool/test.py",
			"etc/other_config.yml",
			"README.md",
		}
		Convey("a project with an empty ignore field should never ignore files", func() {
			p := &Project{Ignore: []string{}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a * ignore field should always ignore files", func() {
			p := &Project{Ignore: []string{"*"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores .py files should not ignore all files", func() {
			p := &Project{Ignore: []string{"*.py"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project that ignores .py, .yml, and .md files should ignore all files", func() {
			p := &Project{Ignore: []string{"*.py", "*.yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by name should ignore all files", func() {
			p := &Project{Ignore: []string{
				"src/cool/test.py",
				"etc/other_config.yml",
				"README.md",
			}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project that ignores all files by dir should ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "etc/*", "README.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeTrue)
		})
		Convey("a project with negations should not ignore all files", func() {
			p := &Project{Ignore: []string{"*", "!src/cool/*"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
		Convey("a project with a negated filetype should not ignore all files", func() {
			p := &Project{Ignore: []string{"src/*", "!*.py", "*yml", "*.md"}}
			So(p.IgnoresAllFiles(files), ShouldBeFalse)
		})
	})
}

func TestPopulateExpansions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(VersionCollection, patch.Collection, ProjectRefCollection,
		task.Collection, ParserProjectCollection))
	defer func() {
		assert.NoError(db.ClearCollections(VersionCollection, patch.Collection, ProjectRefCollection,
			task.Collection, ParserProjectCollection))
	}()

	h := host.Host{
		Id: "h",
		Distro: distro.Distro{
			Id:      "d1",
			WorkDir: "/home/evg",
			Expansions: []distro.Expansion{
				{
					Key:   "note",
					Value: "huge success",
				},
				{
					Key:   "cake",
					Value: "truth",
				},
			},
		},
	}
	projectRef := &ProjectRef{
		Id:         "mci",
		Identifier: "mci-favorite",
	}
	assert.NoError(projectRef.Insert())
	v := &Version{
		Id:                  "v1",
		Branch:              "main",
		Author:              "somebody",
		AuthorEmail:         "somebody@somewhere.com",
		RevisionOrderNumber: 42,
		Requester:           evergreen.GitTagRequester,
		TriggeredByGitTag: GitTag{
			Tag: "release",
		},
	}
	assert.NoError(v.Insert())
	taskDoc := &task.Task{
		Id:           "t1",
		DisplayName:  "magical task",
		Version:      "v1",
		Execution:    0,
		BuildId:      "b1",
		BuildVariant: "magic",
		Revision:     "0ed7cbd33263043fa95aadb3f6068ef8d076854a",
		Project:      "mci",
	}

	expansions, err := PopulateExpansions(taskDoc, &h, "appToken", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 23)
	assert.Equal("0", expansions.Get("execution"))
	assert.Equal("v1", expansions.Get("version_id"))
	assert.Equal("t1", expansions.Get("task_id"))
	assert.Equal("magical task", expansions.Get("task_name"))
	assert.Equal("b1", expansions.Get("build_id"))
	assert.Equal("magic", expansions.Get("build_variant"))
	assert.Equal("0ed7cbd33263043fa95aadb3f6068ef8d076854a", expansions.Get("revision"))
	assert.Equal("0ed7cbd33263043fa95aadb3f6068ef8d076854a", expansions.Get("github_commit"))
	assert.Equal("mci-favorite", expansions.Get("project"))
	assert.Equal("mci", expansions.Get("project_id"))
	assert.Equal("mci-favorite", expansions.Get("project_identifier"))
	assert.Equal("main", expansions.Get("branch_name"))
	assert.Equal("somebody", expansions.Get("author"))
	assert.Equal("somebody@somewhere.com", expansions.Get("author_email"))
	assert.Equal("d1", expansions.Get("distro_id"))
	assert.Equal("release", expansions.Get("triggered_by_git_tag"))
	assert.True(expansions.Exists("created_at"))
	assert.Equal("42", expansions.Get("revision_order_id"))
	assert.Equal("", expansions.Get("is_patch"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.Equal("github_tag", expansions.Get("requester"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.PatchVersionRequester},
	}))
	p := patch.Patch{
		Version: v.Id,
	}
	require.NoError(t, p.Insert())

	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 23)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("patch", expansions.Get("requester"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("triggered_by_git_tag"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.MergeTestRequester},
	}))
	p = patch.Patch{
		Version:     v.Id,
		Description: "commit queue message",
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:       12,
			BaseOwner:      "potato",
			BaseRepo:       "tomato",
			Author:         "hemingway",
			HeadHash:       "7d2fe4649f50f87cb60c2f80ac2ceda1e5b88522",
			MergeCommitSHA: "21",
		},
	}
	require.NoError(t, p.Insert())
	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 29)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("true", expansions.Get("is_commit_queue"))
	assert.Equal("12", expansions.Get("github_pr_number"))
	assert.Equal("potato", expansions.Get("github_org"))
	assert.Equal(p.GithubPatchData.BaseRepo, expansions.Get("github_repo"))
	assert.Equal(p.GithubPatchData.Author, expansions.Get("github_author"))
	assert.Equal(p.GithubPatchData.HeadHash, expansions.Get("github_commit"))
	assert.Equal("commit queue message", expansions.Get("commit_message"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.GithubMergeRequester},
	}))
	p = patch.Patch{
		Version: v.Id,
		GithubMergeData: thirdparty.GithubMergeGroup{
			Org:        "my_merge_org",
			Repo:       "my_merge_repo",
			HeadBranch: "merge_head_branch",
			HeadSHA:    "merge_head_sha",
			HeadCommit: "merge_head_commit",
		},
	}
	require.NoError(t, p.Insert())
	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 27)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("true", expansions.Get("is_commit_queue"))
	assert.Equal("github_merge_queue", expansions.Get("requester"))
	assert.Equal("my_merge_org", expansions.Get("github_org"))
	assert.Equal("my_merge_repo", expansions.Get("github_repo"))
	assert.Equal("merge_head_branch", expansions.Get("github_head_branch"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.GithubPRRequester},
	}))
	p = patch.Patch{
		Version: v.Id,
	}
	require.NoError(t, p.Insert())
	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 27)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("github_pr", expansions.Get("requester"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("triggered_by_git_tag"))
	assert.True(expansions.Exists("github_repo"))
	assert.True(expansions.Exists("github_author"))
	assert.True(expansions.Exists("github_pr_number"))
	assert.True(expansions.Exists("github_commit"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	patchDoc := &patch.Patch{
		Version: v.Id,
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:  42,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			Author:    "octocat",
			HeadHash:  "abc123",
		},
	}
	assert.NoError(patchDoc.Insert())

	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 27)
	assert.Equal("github_pr", expansions.Get("requester"))
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("evergreen", expansions.Get("github_repo"))
	assert.Equal("octocat", expansions.Get("github_author"))
	assert.Equal("42", expansions.Get("github_pr_number"))
	assert.Equal("abc123", expansions.Get("github_commit"))
	assert.Equal("evergreen-ci", expansions.Get("github_org"))

	upstreamTask := task.Task{
		Id:       "upstreamTask",
		Status:   evergreen.TaskFailed,
		Revision: "abc",
		Project:  "upstreamProject",
		Version:  "upstreamVersion",
	}
	assert.NoError(upstreamTask.Insert())
	upstreamProject := ProjectRef{
		Id:     "upstreamProject",
		Branch: "idk",
	}
	assert.NoError(upstreamProject.Insert())
	taskDoc.TriggerID = "upstreamTask"
	taskDoc.TriggerType = ProjectTriggerLevelTask
	expansions, err = PopulateExpansions(taskDoc, &h, "", "")
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 36)
	assert.Equal(taskDoc.TriggerID, expansions.Get("trigger_event_identifier"))
	assert.Equal(taskDoc.TriggerType, expansions.Get("trigger_event_type"))
	assert.Equal(upstreamTask.Revision, expansions.Get("trigger_revision"))
	assert.Equal(upstreamTask.Status, expansions.Get("trigger_status"))
	assert.Equal(upstreamTask.Version, expansions.Get("trigger_version"))
	assert.Equal(upstreamProject.Branch, expansions.Get("trigger_branch"))
}

type projectSuite struct {
	project *Project
	vars    ProjectVars
	aliases []ProjectAlias
	suite.Suite
}

func TestProject(t *testing.T) {
	suite.Run(t, &projectSuite{})
}

func (s *projectSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(ProjectRefCollection, ProjectVarsCollection, fakeparameter.Collection, ProjectAliasCollection, VersionCollection, build.Collection, task.Collection))
	pRef := ProjectRef{
		Id:                    "project",
		ParameterStoreEnabled: true,
	}
	s.Require().NoError(pRef.Insert())
	s.vars = ProjectVars{
		Id: pRef.Id,
	}
	s.Require().NoError(s.vars.Insert())

	s.aliases = []ProjectAlias{
		{
			ProjectID: pRef.Id,
			Alias:     "all",
			Variant:   ".*",
			Task:      ".*",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "bv2",
			Variant:   ".*_2",
			Task:      ".*",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "2tasks",
			Variant:   ".*",
			Task:      ".*_2",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "aTags",
			Variant:   ".*",
			TaskTags:  []string{"a"},
		},
		{
			ProjectID: pRef.Id,
			Alias:     "2tasks(obsolete)", // Remains from times when tags and tasks were allowed together
			Variant:   ".*",
			Task:      ".*_2",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "memes",
			Variant:   ".*",
			Task:      "memes",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "disabled_stuff",
			Variant:   "bv_3",
			Task:      "disabled_.*",
		},
		{
			ProjectID: pRef.Id,
			Alias:     "part_of_memes",
			Variant:   "bv_1",
			TaskTags:  []string{"part_of_memes"},
		},
		{
			ProjectID:   pRef.Id,
			Alias:       "even_bvs",
			VariantTags: []string{"even"},
			TaskTags:    []string{"a"},
		},
	}
	for _, alias := range s.aliases {
		s.NoError(alias.Upsert())
	}
	s.project = &Project{
		Identifier: pRef.Id,
		BuildVariants: []BuildVariant{
			{
				Name: "bv_1",
				Tasks: []BuildVariantTaskUnit{
					{
						Name:    "a_task_1",
						Variant: "bv_1",
						CreateCheckRun: &CheckRun{
							PathToOutputs: "",
						},
					},
					{
						Name:    "a_task_2",
						Variant: "bv_1",
						CreateCheckRun: &CheckRun{
							PathToOutputs: "",
						},
					},
					{
						Name:    "b_task_1",
						Variant: "bv_1",
					},
					{
						Name:    "b_task_2",
						Variant: "bv_1",
					},
					{
						Name:    "9001_task",
						Variant: "bv_1",
						DependsOn: []TaskUnitDependency{
							{
								Name:    "a_task_2",
								Variant: "bv_2",
							},
						},
					},
					{
						Name:    "very_task",
						Variant: "bv_1",
					},
					{
						Name:      "another_disabled_task",
						Variant:   "bv_1",
						Patchable: utility.FalsePtr(),
					},
				},
				DisplayTasks: []patch.DisplayTask{
					{
						Name:      "memes",
						ExecTasks: []string{"9001_task", "very_task", "another_disabled_task"},
					},
				},
			},
			{
				Name: "bv_2",
				Tags: []string{"even"},
				Tasks: []BuildVariantTaskUnit{
					{
						Name:    "a_task_1",
						Variant: "bv_2",
					},
					{
						Name:    "a_task_2",
						Variant: "bv_2",
					},
					{
						Name:    "b_task_1",
						Variant: "bv_2",
					},
					{
						Name:    "b_task_2",
						Variant: "bv_2",
					},
					{
						Name:      "another_disabled_task",
						Variant:   "bv_2",
						Patchable: utility.TruePtr(),
					},
				},
			},
			{
				Name: "bv_3",
				Tasks: []BuildVariantTaskUnit{
					{
						Name:    "disabled_task",
						Variant: "bv_3",
						Disable: utility.TruePtr(),
					},
				},
			},
		},
		Tasks: []ProjectTask{
			{
				Name: "a_task_1",
				Tags: []string{"a", "1"},
			},
			{
				Name: "a_task_2",
				Tags: []string{"a", "2"},
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
					{
						Command: "generate.tasks",
					},
				},
			},
			{
				Name: "b_task_1",
				Tags: []string{"b", "1"},
				Commands: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
					{
						Function: "go generate a thing",
					},
				},
			},
			{
				Name: "b_task_2",
				Tags: []string{"b", "2"},
			},
			{
				Name: "9001_task",
			},
			{
				Name: "very_task",
				Tags: []string{"part_of_memes"},
			},
			{
				Name: "disabled_task",
			},
			{
				Name:      "another_disabled_task",
				Patchable: utility.FalsePtr(),
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"go generate a thing": {
				MultiCommand: []PluginCommandConf{
					{
						Command: "shell.exec",
					},
					{
						Command: "generate.tasks",
					},
					{
						Command: "shell.exec",
					},
				},
			},
		},
	}
}

func (s *projectSuite) TestAliasResolution() {
	// test that .* on variants and tasks selects everything
	pairs, err := s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[0]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 11)
	pairStrs := make([]string, len(pairs.ExecTasks))
	for i, p := range pairs.ExecTasks {
		pairStrs[i] = p.String()
	}
	s.Contains(pairStrs, "bv_1/a_task_1")
	s.Contains(pairStrs, "bv_1/a_task_2")
	s.Contains(pairStrs, "bv_1/b_task_1")
	s.Contains(pairStrs, "bv_1/b_task_2")
	s.Contains(pairStrs, "bv_1/9001_task")
	s.Contains(pairStrs, "bv_1/very_task")
	s.Contains(pairStrs, "bv_2/a_task_1")
	s.Contains(pairStrs, "bv_2/a_task_2")
	s.Contains(pairStrs, "bv_2/b_task_1")
	s.Contains(pairStrs, "bv_2/b_task_2")
	s.Contains(pairStrs, "bv_2/another_disabled_task")
	s.Require().Len(pairs.DisplayTasks, 1)
	s.Equal("bv_1/memes", pairs.DisplayTasks[0].String())

	// test that the .*_2 regex on variants selects just bv_2
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[1]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 5)
	for _, pair := range pairs.ExecTasks {
		s.Equal("bv_2", pair.Variant)
	}
	s.Empty(pairs.DisplayTasks)

	// test that the .*_2 regex on tasks selects just the _2 tasks
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[2]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 4)
	for _, pair := range pairs.ExecTasks {
		s.Contains(pair.TaskName, "task_2")
	}
	s.Empty(pairs.DisplayTasks)

	// test that the 'a' tag only selects 'a' tasks
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[3]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 4)
	for _, pair := range pairs.ExecTasks {
		s.Contains(pair.TaskName, "a_task")
	}
	s.Empty(pairs.DisplayTasks)

	// test that the .*_2 regex selects the union of both
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[4]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 4)
	for _, pair := range pairs.ExecTasks {
		s.NotEqual("b_task_1", pair.TaskName)
	}
	s.Empty(pairs.DisplayTasks)

	// test for display tasks
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[5]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Empty(pairs.ExecTasks)
	s.Require().Len(pairs.DisplayTasks, 1)
	s.Equal("bv_1/memes", pairs.DisplayTasks[0].String())

	// test for alias including a task belong to a disabled variant
	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[6]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Empty(pairs.ExecTasks)
	s.Empty(pairs.DisplayTasks)

	pairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[8]}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Require().Len(pairs.ExecTasks, 2)
	s.Equal("bv_2/a_task_1", pairs.ExecTasks[0].String())
	s.Equal("bv_2/a_task_2", pairs.ExecTasks[1].String())
	s.Empty(pairs.DisplayTasks)
}

func (s *projectSuite) TestCheckRunCount() {
	pairs := TaskVariantPairs{
		ExecTasks: TVPairSet{
			TVPair{
				Variant:  "bv_1",
				TaskName: "a_task_1",
			},
			TVPair{
				Variant:  "bv_1",
				TaskName: "a_task_2",
			},
			TVPair{
				Variant:  "bv_1",
				TaskName: "a_task_3",
			},
		},
	}

	checkRunCount := s.project.GetNumCheckRunsFromTaskVariantPairs(&pairs)
	s.Equal(2, checkRunCount)

	variantTasks := []patch.VariantTasks{
		{
			Variant: "bv_1",
			Tasks:   []string{"a_task_1", "a_task_2", "a_task_3"},
		},
	}
	checkRunCount = s.project.GetNumCheckRunsFromVariantTasks(variantTasks)
	s.Equal(2, checkRunCount)

}
func (s *projectSuite) TestBuildProjectTVPairs() {
	// test all expansions
	patchDoc := patch.Patch{
		BuildVariants: []string{"all"},
		Tasks:         []string{"all"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, evergreen.PatchVersionRequester)

	s.Len(patchDoc.BuildVariants, 2)
	s.ElementsMatch([]string{"bv_1", "bv_2"}, patchDoc.BuildVariants)
	s.Len(patchDoc.Tasks, 7)
	s.ElementsMatch([]string{
		"a_task_1",
		"a_task_2",
		"b_task_1",
		"b_task_2",
		"9001_task",
		"very_task",
		"another_disabled_task"}, patchDoc.Tasks)
	for _, vt := range patchDoc.VariantsTasks {
		switch vt.Variant {
		case "bv_1":
			s.ElementsMatch([]string{
				"a_task_1",
				"a_task_2",
				"b_task_1",
				"b_task_2",
				"9001_task",
				"very_task",
			}, vt.Tasks)
			s.Len(vt.DisplayTasks, 1)
		case "bv_2":
			s.ElementsMatch([]string{
				"a_task_1",
				"a_task_2",
				"b_task_1",
				"b_task_2",
				"another_disabled_task",
			}, vt.Tasks)
			s.Empty(vt.DisplayTasks)
		default:
			s.Fail("unexpected variant '%s'", vt.Variant)
		}
	}

	// test all tasks expansion with named buildvariant expands unnamed buildvariant
	patchDoc.BuildVariants = []string{"bv_1"}
	patchDoc.Tasks = []string{"all"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, evergreen.PatchVersionRequester)

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 6)

	// test all variants expansion with named task
	patchDoc.BuildVariants = []string{"all"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, evergreen.PatchVersionRequester)

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 6)
}

func (s *projectSuite) TestResolvePatchVTs() {
	// Specifying all.
	patchDoc := patch.Patch{
		BuildVariants: []string{"all"},
		Tasks:         []string{"all"},
	}

	bvs, tasks, variantTasks := s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.ElementsMatch([]string{"bv_1", "bv_2"}, bvs)
	s.Len(tasks, 7)
	s.ElementsMatch([]string{
		"a_task_1",
		"a_task_2",
		"b_task_1",
		"b_task_2",
		"9001_task",
		"very_task",
		"another_disabled_task"}, tasks)
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		switch vt.Variant {
		case "bv_1":
			s.ElementsMatch([]string{
				"a_task_1",
				"a_task_2",
				"b_task_1",
				"b_task_2",
				"9001_task",
				"very_task",
			}, vt.Tasks)
			s.Len(vt.DisplayTasks, 1)
		case "bv_2":
			s.ElementsMatch([]string{
				"a_task_1",
				"a_task_2",
				"b_task_1",
				"b_task_2",
				"another_disabled_task",
			}, vt.Tasks)
			s.Empty(vt.DisplayTasks)
		default:
			s.Fail("unexpected variant '%s'", vt.Variant)
		}
	}

	// Build variant and tasks override regex.
	patchDoc = patch.Patch{
		BuildVariants:      []string{"all"},
		Tasks:              []string{"all"},
		RegexBuildVariants: []string{"^bv_"},
		RegexTasks:         []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Len(tasks, 7)
	s.Len(variantTasks, 2)

	// Regex build variants and tasks.
	patchDoc = patch.Patch{
		RegexBuildVariants: []string{".*"},
		RegexTasks:         []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 2)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 2)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

	// Specifying all build variants and giving it regex tasks.
	patchDoc = patch.Patch{
		BuildVariants: []string{"all"},
		RegexTasks:    []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 2)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 2)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

	// Specifying build variants and giving it regex tasks.
	patchDoc = patch.Patch{
		BuildVariants: []string{"bv_1", "bv_2"},
		RegexTasks:    []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 2)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 2)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

	// Alias adds on to the selected regex tasks.
	patchDoc = patch.Patch{
		RegexBuildVariants: []string{".*"},
		RegexTasks:         []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "aTags", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 3)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "a_task_2")
	s.Contains(tasks, "b_task_1")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 3)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "a_task_2")
		s.Contains(vt.Tasks, "b_task_1")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

	// Specifying tags only.
	patchDoc = patch.Patch{
		BuildVariants: []string{".even"},
		Tasks:         []string{".a", ".1"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 1)
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 3)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Contains(tasks, "a_task_2")
	s.Len(variantTasks, 1)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 3)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Contains(vt.Tasks, "a_task_2")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_2"}, vt.Variant)
	}

	// Specifying tags and names.
	patchDoc = patch.Patch{
		BuildVariants: []string{".even", "bv_1"},
		Tasks:         []string{".a", ".1", "b_task_2"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 4)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Contains(tasks, "a_task_2")
	s.Contains(tasks, "b_task_2")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 4)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Contains(vt.Tasks, "a_task_2")
		s.Contains(vt.Tasks, "b_task_2")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

	// Specifying tags, regex, and names.
	patchDoc = patch.Patch{
		BuildVariants: []string{".even", "bv_1"},
		Tasks:         []string{".a"},
		RegexTasks:    []string{"_1$"},
	}

	bvs, tasks, variantTasks = s.project.ResolvePatchVTs(&patchDoc, patchDoc.GetRequester(), "", true)
	s.Len(bvs, 2)
	s.Contains(bvs, "bv_1")
	s.Contains(bvs, "bv_2")
	s.Len(tasks, 3)
	s.Contains(tasks, "a_task_1")
	s.Contains(tasks, "b_task_1")
	s.Contains(tasks, "a_task_2")
	s.Len(variantTasks, 2)
	for _, vt := range variantTasks {
		s.Len(vt.Tasks, 3)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "b_task_1")
		s.Contains(vt.Tasks, "a_task_2")
		s.Empty(vt.DisplayTasks)
		s.Contains([]string{"bv_1", "bv_2"}, vt.Variant)
	}

}

func (s *projectSuite) TestBuildProjectTVPairsWithAlias() {
	patchDoc := patch.Patch{}

	s.project.BuildProjectTVPairs(&patchDoc, "2tasks(obsolete)")

	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "a_task_2")
	s.Contains(patchDoc.Tasks, "b_task_2")

	for _, vt := range patchDoc.VariantsTasks {
		s.Len(vt.Tasks, 2)
		s.Contains(vt.Tasks, "a_task_2")
		s.Contains(vt.Tasks, "b_task_2")
		s.Empty(vt.DisplayTasks)
		if vt.Variant != "bv_1" && vt.Variant != "bv_2" {
			s.T().Fail()
		}
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithBadBuildVariant() {
	patchDoc := patch.Patch{
		BuildVariants: []string{"bv_1", "bv_2", "totallynotreal"},
		Tasks:         []string{"a_task_1", "b_task_1"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Require().Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Contains(patchDoc.Tasks, "b_task_1")
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.VariantsTasks, 2)
	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" || vt.Variant == "bv_2" {
			s.Len(vt.Tasks, 2)
			s.Contains(vt.Tasks, "a_task_1")
			s.Contains(vt.Tasks, "b_task_1")
		} else {
			s.T().Fail()
		}
		s.Empty(vt.DisplayTasks)
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithAliasWithTags() {
	patchDoc := patch.Patch{}

	s.project.BuildProjectTVPairs(&patchDoc, "aTags")

	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Contains(patchDoc.Tasks, "a_task_2")

	for _, vt := range patchDoc.VariantsTasks {
		s.Len(vt.Tasks, 2)
		s.Contains(vt.Tasks, "a_task_1")
		s.Contains(vt.Tasks, "a_task_2")
		if vt.Variant != "bv_1" && vt.Variant != "bv_2" {
			s.T().Fail()
		}
		s.Empty(vt.DisplayTasks)
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithAliasWithDisplayTask() {
	patchDoc := patch.Patch{}

	s.project.BuildProjectTVPairs(&patchDoc, "memes")
	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 3)
	s.Contains(patchDoc.Tasks, "very_task")
	s.Require().Len(patchDoc.VariantsTasks, 2)
	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Contains(vt.Tasks, "very_task")
			s.Require().Len(vt.DisplayTasks, 1)
			s.Equal("memes", vt.DisplayTasks[0].Name)

		} else if vt.Variant == "bv_2" {
			s.Contains(vt.Tasks, "a_task_2")
			s.Empty(vt.DisplayTasks)

		} else {
			s.T().Fail()
		}
	}

}

func (s *projectSuite) TestBuildProjectTVPairsWithDisabledBuildVariant() {
	patchDoc := patch.Patch{}

	s.project.BuildProjectTVPairs(&patchDoc, "disabled_stuff")
	s.Empty(patchDoc.BuildVariants)
	s.Empty(patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)

	patchDoc = patch.Patch{
		BuildVariants: []string{"bv_3"},
		Tasks:         []string{"disabled_task"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, "")
	s.Empty(patchDoc.BuildVariants)
	s.Empty(patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
}

func (s *projectSuite) TestBuildProjectTVPairsWithDisplayTaskWithDependencies() {
	patchDoc := patch.Patch{
		BuildVariants: []string{"bv_1", "bv_2"},
		Tasks:         []string{"memes"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, "")
	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 3)
	s.Contains(patchDoc.Tasks, "very_task")
	s.Contains(patchDoc.Tasks, "a_task_2")
	s.Require().Len(patchDoc.VariantsTasks, 2)

	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Len(vt.Tasks, 2)
			s.Contains(vt.Tasks, "very_task")
			s.Require().Len(vt.DisplayTasks, 1)
			s.Equal("memes", vt.DisplayTasks[0].Name)
			s.Len(vt.DisplayTasks[0].ExecTasks, 3)
			s.Contains(vt.DisplayTasks[0].ExecTasks, "9001_task")
			s.Contains(vt.DisplayTasks[0].ExecTasks, "very_task")
			s.Contains(vt.DisplayTasks[0].ExecTasks, "another_disabled_task")
		} else if vt.Variant == "bv_2" {
			s.Len(vt.Tasks, 1)
			s.Contains(vt.Tasks, "a_task_2")
			s.Empty(vt.DisplayTasks)

		} else {
			s.T().Fail()
		}
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithExecutionTaskFromTags() {
	patchDoc := patch.Patch{}
	s.project.BuildProjectTVPairs(&patchDoc, "part_of_memes")
	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Len(patchDoc.Tasks, 3)
	s.Contains(patchDoc.Tasks, "very_task")
	s.Contains(patchDoc.Tasks, "9001_task")
	s.Contains(patchDoc.Tasks, "a_task_2")
	s.Len(patchDoc.VariantsTasks, 2)
	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Len(vt.Tasks, 2)
			s.Contains(vt.Tasks, "very_task")
			s.Contains(patchDoc.Tasks, "9001_task")
			s.Len(vt.DisplayTasks, 1)
		} else if vt.Variant == "bv_2" {
			s.Len(vt.Tasks, 1)
			s.Contains(vt.Tasks, "a_task_2")
			s.Empty(vt.DisplayTasks)
		}
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithExecutionTask() {
	patchDoc := patch.Patch{
		BuildVariants: []string{"bv_1"},
		Tasks:         []string{"9001_task"},
	}
	s.project.BuildProjectTVPairs(&patchDoc, "")
	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 3)
	s.Contains(patchDoc.Tasks, "9001_task")
	s.Len(patchDoc.VariantsTasks, 2)
}

func (s *projectSuite) TestNewPatchTaskIdTable() {
	p := &Project{
		Identifier: "project_id",
		Tasks: []ProjectTask{
			{
				Name: "task1",
			},
			{
				Name: "task2",
			},
			{
				Name: "task3",
			},
		},
		BuildVariants: []BuildVariant{
			{
				Name:  "test",
				Tasks: []BuildVariantTaskUnit{{Name: "group_1", Variant: "test"}},
			},
		},
		TaskGroups: []TaskGroup{
			{
				Name: "group_1",
				Tasks: []string{
					"task1",
					"task2",
				},
			},
		},
	}
	v := &Version{
		Revision: "revision",
	}
	pairs := TaskVariantPairs{
		ExecTasks: TVPairSet{
			TVPair{
				Variant:  "test",
				TaskName: "group_1",
			},
		},
	}

	config, err := NewTaskIdConfig(p, v, pairs, "project_identifier")
	s.Require().NoError(err)
	s.Len(config.DisplayTasks, 0)
	s.Len(config.ExecutionTasks, 2)
	s.Equal("project_identifier_test_task1_revision_01_01_01_00_00_00",
		config.ExecutionTasks[TVPair{
			Variant:  "test",
			TaskName: "task1",
		}])
	s.Equal("project_identifier_test_task2_revision_01_01_01_00_00_00",
		config.ExecutionTasks[TVPair{
			Variant:  "test",
			TaskName: "task2",
		}])
}

// TestRoundTripIntermediateProjectWithDependsOn ensures that inlining works correctly in depends_on.
func (s *projectSuite) TestRoundTripIntermediateProjectWithDependsOn() {
	projYml := `
tasks:
- name: test
  depends_on:
    - name: dist-test
`
	intermediate, err := createIntermediateProject([]byte(projYml), false)
	s.NoError(err)
	marshaled, err := yaml.Marshal(intermediate)
	s.NoError(err)
	unmarshaled := ParserProject{}
	s.NoError(yaml.Unmarshal(marshaled, &unmarshaled))
}

func (s *projectSuite) TestFetchVersionsBuildsAndTasks() {
	v1 := Version{
		Id:                  "v1",
		Identifier:          s.project.Identifier,
		Revision:            "asdf1",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now(),
		RevisionOrderNumber: 1,
	}
	s.NoError(v1.Insert())
	v2 := Version{
		Id:                  "v2",
		Identifier:          s.project.Identifier,
		Revision:            "asdf2",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now().Add(1 * time.Minute),
		RevisionOrderNumber: 2,
	}
	s.NoError(v2.Insert())
	v3 := Version{
		Id:                  "v3",
		Identifier:          s.project.Identifier,
		Revision:            "asdf3",
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now().Add(5 * time.Minute),
		RevisionOrderNumber: 3,
	}
	s.NoError(v3.Insert())
	b1 := build.Build{
		Id:       "b1",
		Version:  v1.Id,
		Revision: v1.Revision,
		Tasks:    []build.TaskCache{{Id: "t1"}, {Id: "t2"}},
	}
	s.NoError(b1.Insert())
	b2 := build.Build{
		Id:       "b2",
		Version:  v2.Id,
		Revision: v2.Revision,
	}
	s.NoError(b2.Insert())
	b3 := build.Build{
		Id:       "b3",
		Version:  v3.Id,
		Revision: v3.Revision,
	}
	s.NoError(b3.Insert())
	t1 := task.Task{
		Id:      "t1",
		BuildId: b1.Id,
		Version: v1.Id,
	}
	s.NoError(t1.Insert())
	t2 := task.Task{
		Id:      "t2",
		BuildId: b1.Id,
		Version: v1.Id,
	}
	s.NoError(t2.Insert())

	versions, builds, tasks, err := FetchVersionsBuildsAndTasks(s.project, 0, 10, false)
	s.NoError(err)
	s.Equal(v3.Id, versions[0].Id)
	s.Equal(v2.Id, versions[1].Id)
	s.Equal(v1.Id, versions[2].Id)
	s.Equal(b1.Id, builds[v1.Id][0].Id)
	s.Equal(b2.Id, builds[v2.Id][0].Id)
	s.Equal(b3.Id, builds[v3.Id][0].Id)
	s.Equal(v1.Revision, builds[v1.Id][0].Revision)
	s.Equal(v2.Revision, builds[v2.Id][0].Revision)
	s.Equal(v3.Revision, builds[v3.Id][0].Revision)
	s.Equal(t1.Id, tasks[b1.Id][0].Id)
	s.Equal(t2.Id, tasks[b1.Id][1].Id)
}

func (s *projectSuite) TestIsGenerateTask() {
	s.False(s.project.IsGenerateTask("a_task_1"))
	s.True(s.project.IsGenerateTask("a_task_2"))
	s.True(s.project.IsGenerateTask("b_task_1"))
	s.False(s.project.IsGenerateTask("b_task_2"))
	s.False(s.project.IsGenerateTask("9001_task"))
	s.False(s.project.IsGenerateTask("very_task"))
	s.False(s.project.IsGenerateTask("disabled_task"))
	s.False(s.project.IsGenerateTask("another_disabled_task"))
	s.False(s.project.IsGenerateTask("task_does_not_exist"))
}

func (s *projectSuite) TestFindAllTasksMap() {
	allTasks := s.project.FindAllTasksMap()
	s.Len(allTasks, 8)
	task := allTasks["a_task_1"]
	s.Len(task.Tags, 2)
}

type FindProjectsSuite struct {
	setup    func() error
	teardown func() error
	suite.Suite
}

const (
	repoProjectId  = "repo_mci"
	projEventCount = 10
)

func TestFindProjectsSuite(t *testing.T) {
	s := new(FindProjectsSuite)
	s.setup = func() error {
		s.Require().NoError(db.ClearCollections(ProjectRefCollection, ProjectVarsCollection, fakeparameter.Collection))

		projectWithVars := &ProjectRef{
			Id:                    projectId,
			ParameterStoreEnabled: true,
		}
		projects := []*ProjectRef{
			{
				Id:                    "projectA",
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "gimlet",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectB",
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "evergreen",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectC",
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "mongodb",
				Repo:                  "mongo",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectA-hidden",
				Hidden:                utility.TruePtr(),
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "gimlet",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectB-hidden",
				Hidden:                utility.TruePtr(),
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "evergreen-ci",
				Repo:                  "evergreen",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectC-hidden",
				Hidden:                utility.TruePtr(),
				Enabled:               true,
				CommitQueue:           CommitQueueParams{Enabled: utility.TruePtr()},
				Owner:                 "mongodb",
				Repo:                  "mongo",
				Branch:                "main",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectD",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectE",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectF",
				ParameterStoreEnabled: true,
			},
			{
				Id:                    "projectF-hidden",
				Hidden:                utility.TruePtr(),
				ParameterStoreEnabled: true,
			},
			projectWithVars,
		}

		for _, p := range projects {
			if err := p.Insert(); err != nil {
				return err
			}
			if _, err := GetNewRevisionOrderNumber(p.Id); err != nil {
				return err
			}
		}

		vars := &ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{"a": "1", "b": "3"},
			PrivateVars: map[string]bool{"b": true},
		}
		s.NoError(vars.Insert())
		checkAndSetProjectVarsSynced(s.T(), projectWithVars, false)
		checkParametersNamespacedByProject(s.T(), *vars)

		repoWithVars := &RepoRef{ProjectRef{
			Id:                    repoProjectId,
			ParameterStoreEnabled: true,
		},
		}
		s.Require().NoError(repoWithVars.Upsert())
		repoVars := &ProjectVars{
			Id:          repoProjectId,
			Vars:        map[string]string{"a": "a_from_repo", "c": "new"},
			PrivateVars: map[string]bool{"a": true},
		}
		s.NoError(repoVars.Insert())
		checkAndSetProjectVarsSynced(s.T(), &repoWithVars.ProjectRef, true)
		checkParametersNamespacedByProject(s.T(), *repoVars)

		before := getMockProjectSettings()
		after := getMockProjectSettings()
		after.GithubHooksEnabled = false

		h :=
			event.EventLogEntry{
				Timestamp:    time.Now(),
				ResourceType: event.EventResourceTypeProject,
				EventType:    event.EventTypeProjectModified,
				ResourceId:   projectId,
				Data: &ProjectChangeEvent{
					User:   username,
					Before: NewProjectSettingsEvent(before),
					After:  NewProjectSettingsEvent(after),
				},
			}

		s.Require().NoError(db.ClearCollections(event.EventCollection))
		for i := 0; i < projEventCount; i++ {
			eventShallowCpy := h
			s.NoError(eventShallowCpy.Log())
		}

		return nil
	}

	s.teardown = func() error {
		return db.Clear(ProjectRefCollection)
	}

	suite.Run(t, s)
}

func (s *FindProjectsSuite) SetupSuite() { s.Require().NoError(s.setup()) }

func (s *FindProjectsSuite) TearDownSuite() {
	s.Require().NoError(s.teardown())
}

func (s *FindProjectsSuite) TestFetchTooManyAsc() {
	projects, err := FindNonHiddenProjects("", 8, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *FindProjectsSuite) TestFetchTooManyDesc() {
	projects, err := FindNonHiddenProjects("zzz", 8, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 7)
}

func (s *FindProjectsSuite) TestFetchExactNumber() {
	projects, err := FindNonHiddenProjects("", 3, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 3)
}

func (s *FindProjectsSuite) TestFetchTooFewAsc() {
	projects, err := FindNonHiddenProjects("", 2, 1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *FindProjectsSuite) TestFetchTooFewDesc() {
	projects, err := FindNonHiddenProjects("zzz", 2, -1)
	s.NoError(err)
	s.NotNil(projects)
	s.Len(projects, 2)
}

func (s *FindProjectsSuite) TestFetchKeyWithinBoundAsc() {
	projects, err := FindNonHiddenProjects("projectB", 1, 1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *FindProjectsSuite) TestFetchKeyWithinBoundDesc() {
	projects, err := FindNonHiddenProjects("projectD", 1, -1)
	s.NoError(err)
	s.Len(projects, 1)
}

func (s *FindProjectsSuite) TestFetchKeyOutOfBoundAsc() {
	projects, err := FindNonHiddenProjects("zzz", 1, 1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *FindProjectsSuite) TestFetchKeyOutOfBoundDesc() {
	projects, err := FindNonHiddenProjects("aaa", 1, -1)
	s.NoError(err)
	s.Len(projects, 0)
}

func (s *FindProjectsSuite) TestGetProjectWithCommitQueueByOwnerRepoAndBranch() {
	projRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("octocat", "hello-world", "main")
	s.NoError(err)
	s.Nil(projRef)

	projRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("evergreen-ci", "evergreen", "main")
	s.NoError(err)
	s.NotNil(projRef)
}

func (s *FindProjectsSuite) TestGetProjectSettings() {
	projRef := &ProjectRef{
		Owner:   "admin",
		Enabled: true,
		Id:      projectId,
		Admins:  []string{},
		Repo:    "SomeRepo",
	}
	projectSettingsEvent, err := GetProjectSettings(projRef)
	s.NoError(err)
	s.NotNil(projectSettingsEvent)
}

func (s *FindProjectsSuite) TestGetProjectSettingsNoRepo() {
	projRef := &ProjectRef{
		Owner:   "admin",
		Enabled: true,
		Id:      projectId,
		Admins:  []string{},
	}
	projectSettingsEvent, err := GetProjectSettings(projRef)
	s.Nil(err)
	s.NotNil(projectSettingsEvent)
	s.False(projectSettingsEvent.GithubHooksEnabled)
}

func TestModuleList(t *testing.T) {
	assert := assert.New(t)

	projModules := ModuleList{
		{Name: "enterprise", Owner: "something", Repo: "enterprise", Branch: "main"},
		{Name: "wt", Owner: "else", Repo: "wt", Branch: "develop"},
	}

	manifest1 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         {Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": {Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.True(projModules.IsIdentical(manifest1))

	manifest2 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         {Branch: "different branch", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": {Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(projModules.IsIdentical(manifest2))

	manifest3 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         {Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": {Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
			"extra":      {Branch: "main", Repo: "repo", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(projModules.IsIdentical(manifest3))

	manifest4 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt": {Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
		},
	}
	assert.False(projModules.IsIdentical(manifest4))
}

func TestInjectTaskGroupInfo(t *testing.T) {
	tg := TaskGroup{
		Name:     "group-one",
		MaxHosts: 42,
		Tasks:    []string{"one", "two"},
	}

	t.Run("PopulatedFirst", func(t *testing.T) {
		tk := &task.Task{
			DisplayName: "one",
		}

		tg.InjectInfo(tk)

		assert.Equal(t, 42, tk.TaskGroupMaxHosts)
		assert.Equal(t, 1, tk.TaskGroupOrder)
	})
	t.Run("PopulatedSecond", func(t *testing.T) {
		tk := &task.Task{
			DisplayName: "two",
		}

		tg.InjectInfo(tk)

		assert.Equal(t, 42, tk.TaskGroupMaxHosts)
		assert.Equal(t, 2, tk.TaskGroupOrder)
	})
	t.Run("Missed", func(t *testing.T) {
		tk := &task.Task{}

		tg.InjectInfo(tk)

		assert.Equal(t, 42, tk.TaskGroupMaxHosts)
		assert.Equal(t, 0, tk.TaskGroupOrder)
	})
}

func TestCommandsRunOnTV(t *testing.T) {
	cmd := evergreen.S3PushCommandName
	for testName, testCase := range map[string]struct {
		project      Project
		tv           TVPair
		expectError  bool
		expectedCmds []PluginCommandConf
	}{
		"FindsTaskWithCommand": {
			project: Project{
				Tasks: []ProjectTask{
					{
						Name: "task",
						Commands: []PluginCommandConf{
							{
								Command:     cmd,
								DisplayName: "display_name",
							},
						},
					},
				},
			},
			tv: TVPair{TaskName: "task", Variant: "variant"},
			expectedCmds: []PluginCommandConf{
				{
					Command:     cmd,
					DisplayName: "display_name",
				},
			},
		},
		"FailsForCommandsInTaskButFiltered": {
			project: Project{
				Tasks: []ProjectTask{
					{
						Name: "task",
						Commands: []PluginCommandConf{
							{
								Command:     cmd,
								Variants:    []string{"other_variant"},
								DisplayName: "display_name",
							},
						},
					},
				},
			},
			tv: TVPair{TaskName: "task", Variant: "variant"},
		},
		"FailsForNonexistentTaskDefinition": {
			project:     Project{},
			tv:          TVPair{TaskName: "task", Variant: "variant"},
			expectError: true,
		},
		"FailsForCommandNotInTask": {
			project: Project{
				Tasks: []ProjectTask{
					{Name: "task"},
				},
			},
			tv: TVPair{TaskName: "task", Variant: "variant"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			cmds, err := testCase.project.CommandsRunOnTV(testCase.tv, cmd)
			if testCase.expectError {
				assert.Error(t, err)
				assert.Empty(t, cmds)
			} else {
				assert.NoError(t, err)
				assert.Len(t, cmds, len(testCase.expectedCmds))
				expectedFound := map[string]PluginCommandConf{}
				for _, expectedCmd := range testCase.expectedCmds {
					expectedFound[expectedCmd.DisplayName] = PluginCommandConf{}
				}
				for _, cmd := range cmds {
					_, ok := expectedFound[cmd.DisplayName]
					if assert.True(t, ok, "unexpected command '%s' in result", cmd.DisplayName) {
						expectedFound[cmd.DisplayName] = cmd
					}
				}
				for _, expectedCmd := range testCase.expectedCmds {
					actualCmd := expectedFound[expectedCmd.DisplayName]
					assert.Equal(t, expectedCmd.DisplayName, actualCmd.DisplayName)
					assert.Equal(t, expectedCmd.Command, actualCmd.Command)
				}
			}
		})
	}
}

func TestCommandsRunOnBV(t *testing.T) {
	cmd := evergreen.S3PullCommandName
	variant := "variant"
	for testName, testCase := range map[string]struct {
		expectedCmdNames []string
		cmds             []PluginCommandConf
		variant          string
		funcs            map[string]*YAMLCommandSet
	}{
		"FindsMatchingCommands": {
			cmds: []PluginCommandConf{
				{
					Command:     cmd,
					DisplayName: "display",
				}, {
					Command: "foo",
				},
			},
			variant:          variant,
			expectedCmdNames: []string{"display"},
		},
		"FindsMatchingCommandsInFunction": {
			cmds: []PluginCommandConf{
				{
					Function: "function",
				},
			},
			funcs: map[string]*YAMLCommandSet{
				"function": {
					SingleCommand: &PluginCommandConf{
						Command:     cmd,
						DisplayName: "display",
					},
				},
			},
			expectedCmdNames: []string{"display"},
			variant:          variant,
		},
		"FindsMatchingCommandsFilteredByVariant": {
			cmds: []PluginCommandConf{
				{
					Command:     cmd,
					DisplayName: "display1",
				}, {
					Command:  cmd,
					Variants: []string{"other_variant"},
				}, {
					Command:     cmd,
					DisplayName: "display2",
					Variants:    []string{variant},
				}, {
					Command:     cmd,
					DisplayName: "display3",
					Variants:    []string{"other_variant", variant},
				},
			},
			expectedCmdNames: []string{"display1", "display2", "display3"},
			variant:          variant,
		},
		"FindsMatchingCommandsInFunctionFilteredByVariant": {
			cmds: []PluginCommandConf{
				{
					Function: "function",
				}, {
					Command:  cmd,
					Variants: []string{"other_variant"},
				}, {
					Command:     cmd,
					DisplayName: "display2",
					Variants:    []string{variant},
				}, {
					Command:     cmd,
					DisplayName: "display3",
					Variants:    []string{"other_variant", variant},
				},
			},
			funcs: map[string]*YAMLCommandSet{
				"function": {
					SingleCommand: &PluginCommandConf{
						Command:     cmd,
						DisplayName: "display1",
					},
				},
			},
			expectedCmdNames: []string{"display1", "display2", "display3"},
			variant:          variant,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			p := &Project{Functions: testCase.funcs}
			cmds := p.CommandsRunOnBV(testCase.cmds, cmd, variant)
			assert.Len(t, cmds, len(testCase.expectedCmdNames))
			for _, cmd := range cmds {
				assert.True(t, utility.StringSliceContains(testCase.expectedCmdNames, cmd.DisplayName), "unexpected command '%s'", cmd.DisplayName)
			}
		})
	}
}

func TestGetAllVariantTasks(t *testing.T) {
	for testName, testCase := range map[string]struct {
		project  Project
		expected []patch.VariantTasks
	}{
		"IncludesAllTasksInBVs": {
			project: Project{
				BuildVariants: []BuildVariant{
					{
						Name: "bv1",
						Tasks: []BuildVariantTaskUnit{
							{Name: "t1", Variant: "bv1"},
							{Name: "t2", Variant: "bv1"},
						},
					}, {
						Name: "bv2",
						Tasks: []BuildVariantTaskUnit{
							{Name: "t2", Variant: "bv2"},
							{Name: "t3", Variant: "bv2"},
						},
					},
				},
			},
			expected: []patch.VariantTasks{
				{
					Variant: "bv1",
					Tasks:   []string{"t1", "t2"},
				}, {
					Variant: "bv2",
					Tasks:   []string{"t2", "t3"},
				},
			},
		},
		"IncludesAllDisplayTasksInBVs": {
			project: Project{
				BuildVariants: []BuildVariant{
					{
						Name: "bv1",
						DisplayTasks: []patch.DisplayTask{
							{
								Name:      "dt1",
								ExecTasks: []string{"et1", "et2"},
							},
						},
					}, {
						Name: "bv2",
						DisplayTasks: []patch.DisplayTask{
							{
								Name:      "dt2",
								ExecTasks: []string{"et2", "et3"},
							},
						},
					},
				},
			},
			expected: []patch.VariantTasks{
				{
					Variant: "bv1",
					DisplayTasks: []patch.DisplayTask{
						{
							Name:      "dt1",
							ExecTasks: []string{"et1", "et2"},
						},
					},
				}, {
					Variant: "bv2",
					DisplayTasks: []patch.DisplayTask{
						{
							Name:      "dt2",
							ExecTasks: []string{"et2", "et3"},
						},
					},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			vts := testCase.project.GetAllVariantTasks()
			checkEqualVTs(t, testCase.expected, vts)
		})
	}
}

// checkEqualVT checks that the two VariantTasks are identical.
func checkEqualVT(t *testing.T, expected patch.VariantTasks, actual patch.VariantTasks) {
	missingExpected, missingActual := utility.StringSliceSymmetricDifference(expected.Tasks, actual.Tasks)
	assert.Empty(t, missingExpected, "unexpected tasks '%s' for build variant'%s'", missingExpected, expected.Variant)
	assert.Empty(t, missingActual, "missing expected tasks '%s' for build variant '%s'", missingActual, actual.Variant)

	expectedDTs := map[string]patch.DisplayTask{}
	for _, dt := range expected.DisplayTasks {
		expectedDTs[dt.Name] = dt
	}
	actualDTs := map[string]patch.DisplayTask{}
	for _, dt := range actual.DisplayTasks {
		actualDTs[dt.Name] = dt
	}
	assert.Len(t, actualDTs, len(expectedDTs))
	for _, expectedDT := range expectedDTs {
		actualDT, ok := actualDTs[expectedDT.Name]
		if !assert.True(t, ok, "display task '%s'") {
			continue
		}
		missingExpected, missingActual = utility.StringSliceSymmetricDifference(expectedDT.ExecTasks, actualDT.ExecTasks)
		assert.Empty(t, missingExpected, "unexpected exec tasks '%s' for display task '%s' in build variant '%s'", missingExpected, expectedDT.Name, expected.Variant)
		assert.Empty(t, missingActual, "missing exec tasks '%s' for display task '%s' in build variant '%s'", missingActual, actualDT.Name, actual.Variant)
	}
}

// checkEqualVTs checks that the two slices of VariantTasks are identical sets.
func checkEqualVTs(t *testing.T, expected []patch.VariantTasks, actual []patch.VariantTasks) {
	assert.Len(t, actual, len(expected))
	for _, expectedVT := range expected {
		var found bool
		for _, actualVT := range actual {
			if actualVT.Variant != expectedVT.Variant {
				continue
			}
			found = true
			checkEqualVT(t, expectedVT, actualVT)
			break
		}
		assert.True(t, found, "build variant '%s' not found", expectedVT.Variant)
	}
}

func TestExtractDisplayTasks(t *testing.T) {
	p := Project{
		BuildVariants: []BuildVariant{
			{
				Name: "bv0",
				DisplayTasks: []patch.DisplayTask{
					{Name: "dt0", ExecTasks: []string{"dt0_et0", "dt0_et1"}},
					{Name: "dt1", ExecTasks: []string{"dt1_et0", "dt1_et1"}},
				},
			}},
	}

	incomingPairs := TaskVariantPairs{
		DisplayTasks: TVPairSet{{Variant: "bv0", TaskName: "dt0"}},
		ExecTasks:    TVPairSet{{Variant: "bv0", TaskName: "dt1_et0"}},
	}

	resultingPairs := p.extractDisplayTasks(incomingPairs)

	expectedDisplayTasks := []string{"dt0", "dt1"}
	expectedExecTasks := []string{"dt0_et0", "dt0_et1", "dt1_et0", "dt1_et1"}
	assert.Len(t, resultingPairs.DisplayTasks, len(expectedDisplayTasks))
	assert.Len(t, resultingPairs.ExecTasks, len(expectedExecTasks))
	for _, tvPair := range resultingPairs.DisplayTasks {
		assert.Contains(t, expectedDisplayTasks, tvPair.TaskName)
	}
	for _, tvPair := range resultingPairs.ExecTasks {
		assert.Contains(t, expectedExecTasks, tvPair.TaskName)
	}
}

func TestVariantTasksForSelectors(t *testing.T) {
	require.NoError(t, db.Clear(ProjectAliasCollection))
	projectID := "mci"
	alias := "alias"
	patchAlias := ProjectAlias{
		ProjectID: projectID,
		Alias:     alias,
		Variant:   "bv0",
		Task:      "t0",
	}
	require.NoError(t, patchAlias.Upsert())

	project := Project{
		Identifier: projectID,
		BuildVariants: []BuildVariant{
			{
				Name:         "bv0",
				DisplayTasks: []patch.DisplayTask{{Name: "dt0", ExecTasks: []string{"t0"}}},
				Tasks: []BuildVariantTaskUnit{
					{Name: "t0", Variant: "bv0"},
					{Name: "t1", Variant: "bv0", DependsOn: []TaskUnitDependency{{Name: "t0", Variant: "bv0"}}}},
			},
		},
		Tasks: []ProjectTask{
			{Name: "t0"},
			{Name: "t1", DependsOn: []TaskUnitDependency{{Name: "t0", Variant: "bv0"}}},
		},
	}

	for testName, test := range map[string]func(*testing.T){
		"patch alias selector": func(t *testing.T) {
			definitions := []patch.PatchTriggerDefinition{{TaskSpecifiers: []patch.TaskSpecifier{{PatchAlias: alias}}}}
			vts, err := project.VariantTasksForSelectors(definitions, "")
			assert.NoError(t, err)
			require.Len(t, vts, 1)
			require.Len(t, vts[0].Tasks, 1)
			assert.Equal(t, vts[0].Tasks[0], "t0")
		},
		"selector with dependency": func(t *testing.T) {
			definitions := []patch.PatchTriggerDefinition{{TaskSpecifiers: []patch.TaskSpecifier{{VariantRegex: "bv0", TaskRegex: "t1"}}}}
			vts, err := project.VariantTasksForSelectors(definitions, "")
			assert.NoError(t, err)
			require.Len(t, vts, 1)
			require.Len(t, vts[0].Tasks, 2)
			assert.Contains(t, vts[0].Tasks, "t0")
			assert.Contains(t, vts[0].Tasks, "t1")
		},
		"selector with display task": func(t *testing.T) {
			definitions := []patch.PatchTriggerDefinition{{TaskSpecifiers: []patch.TaskSpecifier{{VariantRegex: "bv0", TaskRegex: "dt0"}}}}
			vts, err := project.VariantTasksForSelectors(definitions, "")
			assert.NoError(t, err)
			require.Len(t, vts, 1)
			require.Len(t, vts[0].Tasks, 1)
			assert.Contains(t, vts[0].Tasks, "t0")
			require.Len(t, vts[0].DisplayTasks, 1)
			assert.Equal(t, vts[0].DisplayTasks[0].Name, "dt0")
		},
	} {
		t.Run(testName, test)
	}
}

func TestSkipOnRequester(t *testing.T) {
	t.Run("PatchRequester", func(t *testing.T) {
		requester := evergreen.PatchVersionRequester
		userRequester := evergreen.PatchVersionUserRequester

		t.Run("Runnable", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("PatchableFalse", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{Patchable: utility.FalsePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("GitTagOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{GitTagOnly: utility.TruePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{userRequester}}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("NotAllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GitTagUserRequester}}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequesterHasHigherPrecedenceThanPatchableFalse", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{
				Patchable:         utility.FalsePtr(),
				AllowedRequesters: []evergreen.UserRequester{userRequester},
			}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
	})

	t.Run("NonPatchRequester", func(t *testing.T) {
		requester := evergreen.RepotrackerVersionRequester
		userRequester := evergreen.RepotrackerVersionUserRequester

		t.Run("Runnable", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("PatchOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{PatchOnly: utility.TruePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("GitTagOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{GitTagOnly: utility.TruePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{userRequester}}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("NotAllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.PatchVersionUserRequester}}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequesterHasHigherPrecedenceThanPatchOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{
				PatchOnly:         utility.TruePtr(),
				AllowedRequesters: []evergreen.UserRequester{userRequester},
			}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
	})

	t.Run("GitTagRequester", func(t *testing.T) {
		requester := evergreen.GitTagRequester
		userRequester := evergreen.GitTagUserRequester

		t.Run("Runnable", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("PatchOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{PatchOnly: utility.TruePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowForGitTagFalse", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowForGitTag: utility.FalsePtr()}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{userRequester}}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
		t.Run("NotAllowedRequester", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.PatchVersionUserRequester}}
			assert.True(t, bvt.SkipOnRequester(requester))
		})
		t.Run("AllowedRequesterHasHigherPrecedenceThanPatchOnly", func(t *testing.T) {
			bvt := BuildVariantTaskUnit{
				PatchOnly:         utility.TruePtr(),
				AllowedRequesters: []evergreen.UserRequester{userRequester},
			}
			assert.False(t, bvt.SkipOnRequester(requester))
		})
	})
}

func TestSkipOnPatchBuild(t *testing.T) {
	t.Run("ReturnsFalseByDefault", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{}
		assert.False(t, bvt.SkipOnPatchBuild())
	})
	t.Run("ReturnsFalseWithPatchable", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{Patchable: utility.TruePtr()}
		assert.False(t, bvt.SkipOnPatchBuild())
	})
	t.Run("ReturnsTrueWithNotPatchable", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{Patchable: utility.FalsePtr()}
		assert.True(t, bvt.SkipOnPatchBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GithubPRUserRequester, evergreen.AdHocUserRequester}}
		assert.False(t, bvt.SkipOnPatchBuild())
	})
	t.Run("ReturnsTrueWithNotAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}}
		assert.True(t, bvt.SkipOnPatchBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequesterAndNotPatchable", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{Patchable: utility.FalsePtr(), AllowedRequesters: []evergreen.UserRequester{evergreen.GithubPRUserRequester}}
		assert.False(t, bvt.SkipOnPatchBuild())
	})
}

func TestSkipOnNonPatchBuild(t *testing.T) {
	t.Run("ReturnsFalseByDefault", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{}
		assert.False(t, bvt.SkipOnNonPatchBuild())
	})
	t.Run("ReturnsFalseWithNotPatchOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{PatchOnly: utility.FalsePtr()}
		assert.False(t, bvt.SkipOnNonPatchBuild())
	})
	t.Run("ReturnsTrueWithPatchOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{PatchOnly: utility.TruePtr()}
		assert.True(t, bvt.SkipOnNonPatchBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GithubPRUserRequester, evergreen.AdHocUserRequester}}
		assert.False(t, bvt.SkipOnNonPatchBuild())
	})
	t.Run("ReturnsTrueWithNotAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GithubPRUserRequester}}
		assert.True(t, bvt.SkipOnNonPatchBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequesterAndPatchOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{PatchOnly: utility.TruePtr(), AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}}
		assert.False(t, bvt.SkipOnNonPatchBuild())
	})
}

func TestSkipOnGitTagBuild(t *testing.T) {
	t.Run("ReturnsFalseByDefault", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{}
		assert.False(t, bvt.SkipOnGitTagBuild())
	})
	t.Run("ReturnsFalseWithGitTagAllowed", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowForGitTag: utility.TruePtr()}
		assert.False(t, bvt.SkipOnGitTagBuild())
	})
	t.Run("ReturnsTrueWithNotGitTagAllowed", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowForGitTag: utility.FalsePtr()}
		assert.True(t, bvt.SkipOnGitTagBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GitTagUserRequester}}
		assert.False(t, bvt.SkipOnGitTagBuild())
	})
	t.Run("ReturnsTrueWithNotAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}}
		assert.True(t, bvt.SkipOnGitTagBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequesterAndGitTagNotAllowed", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowForGitTag: utility.FalsePtr(), AllowedRequesters: []evergreen.UserRequester{evergreen.GitTagUserRequester}}
		assert.False(t, bvt.SkipOnGitTagBuild())
	})
}

func TestSkipOnNonGitTagBuild(t *testing.T) {
	t.Run("ReturnsFalseByDefault", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{}
		assert.False(t, bvt.SkipOnNonGitTagBuild())
	})
	t.Run("ReturnsFalseWithNotGitTagOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{GitTagOnly: utility.FalsePtr()}
		assert.False(t, bvt.SkipOnNonGitTagBuild())
	})
	t.Run("ReturnsTrueWithGitTagOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{GitTagOnly: utility.TruePtr()}
		assert.True(t, bvt.SkipOnNonGitTagBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}}
		assert.False(t, bvt.SkipOnNonGitTagBuild())
	})
	t.Run("ReturnsTrueWithNotAllowedRequester", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{AllowedRequesters: []evergreen.UserRequester{evergreen.GitTagUserRequester}}
		assert.True(t, bvt.SkipOnNonGitTagBuild())
	})
	t.Run("ReturnsFalseWithAllowedRequesterAndGitTagOnly", func(t *testing.T) {
		bvt := BuildVariantTaskUnit{GitTagOnly: utility.TruePtr(), AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}}
		assert.False(t, bvt.SkipOnNonGitTagBuild())
	})
}

func TestDependencyGraph(t *testing.T) {
	p := Project{
		BuildVariants: []BuildVariant{
			{
				Name: "ubuntu",
				Tasks: []BuildVariantTaskUnit{
					{Name: "compile", Variant: "ubuntu", DependsOn: []TaskUnitDependency{{Name: "setup"}}},
					{Name: "setup", Variant: "ubuntu"},
				},
			},
			{
				Name: "rhel",
				Tasks: []BuildVariantTaskUnit{
					{Name: "compile", Variant: "rhel", DependsOn: []TaskUnitDependency{{Name: "setup"}}},
					{Name: "setup", Variant: "rhel"},
				},
			},
		},
	}
	depGraph := p.DependencyGraph()
	assert.NotNil(t, depGraph.GetDependencyEdge(task.TaskNode{Name: "compile", Variant: "ubuntu"}, task.TaskNode{Name: "setup", Variant: "ubuntu"}))
	assert.NotNil(t, depGraph.GetDependencyEdge(task.TaskNode{Name: "compile", Variant: "rhel"}, task.TaskNode{Name: "setup", Variant: "rhel"}))
}

func TestFindAllBuildVariantTasks(t *testing.T) {
	t.Run("TaskGroup", func(t *testing.T) {
		tasks := []ProjectTask{
			{Name: "in_group_0"},
			{Name: "in_group_1"},
		}
		const bvName = "bv"
		const tgName = "task_group"
		bvTasks := []BuildVariantTaskUnit{{Name: tgName, IsGroup: true, Variant: bvName}}
		groups := []TaskGroup{{Name: bvTasks[0].Name, Tasks: []string{tasks[0].Name, tasks[1].Name}}}
		p := Project{
			Tasks:         tasks,
			BuildVariants: []BuildVariant{{Name: bvName, Tasks: bvTasks}},
			TaskGroups:    groups,
		}

		bvts := p.FindAllBuildVariantTasks()
		require.Len(t, bvts, len(tasks))
		for i, bvtu := range bvts {
			assert.Equal(t, tasks[i].Name, bvtu.Name)
			assert.Equal(t, bvName, bvtu.Variant)
			assert.False(t, bvtu.IsGroup)
			assert.True(t, bvtu.IsPartOfGroup)
			assert.Equal(t, tgName, bvtu.GroupName)
		}
	})
}

func TestDependenciesForTaskUnit(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expectedDependencies []task.DependencyEdge
		taskUnits            []BuildVariantTaskUnit
	}{
		"WithExplicitVariants": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "rhel"}},
			},
		},
		"WithDependencyVariantsBasedOnTaskUnit": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name: "setup",
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "compile", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "ubuntu"}},
			},
		},
		"WithOneTaskAndAllVariants": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name:    "setup",
							Variant: AllVariants,
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "compile", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "ubuntu"}},
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "rhel"}},
			},
		},
		"WithAllTasksAndOneVariant": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name:    AllDependencies,
							Variant: "rhel",
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "compile", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "rhel"}},
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "compile", Variant: "rhel"}},
			},
		},
		"WithAllTasksAndOneVariantBasedOnTaskUnit": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name: AllDependencies,
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "compile", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "ubuntu"}},
			},
		},
		"WithAllTasksAndAllVariants": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name:    AllDependencies,
							Variant: AllVariants,
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "compile", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "ubuntu"}},
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "rhel"}},
				{From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "compile", Variant: "rhel"}},
			},
		},
		"ByStatus": {
			taskUnits: []BuildVariantTaskUnit{
				{
					Name:    "compile",
					Variant: "ubuntu",
					DependsOn: []TaskUnitDependency{
						{
							Name:    "setup",
							Variant: "rhel",
							Status:  evergreen.TaskSucceeded,
						},
						{
							Name:    "setup",
							Variant: "ubuntu",
							Status:  task.AllStatuses,
						},
					},
				},
				{Name: "setup", Variant: "rhel"},
				{Name: "setup", Variant: "ubuntu"},
			},
			expectedDependencies: []task.DependencyEdge{
				{Status: evergreen.TaskSucceeded, From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "rhel"}},
				{Status: task.AllStatuses, From: task.TaskNode{Name: "compile", Variant: "ubuntu"}, To: task.TaskNode{Name: "setup", Variant: "ubuntu"}},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			dependencies := dependenciesForTaskUnit(testCase.taskUnits)
			assert.Len(t, dependencies, len(testCase.expectedDependencies))
			for _, expectedDep := range testCase.expectedDependencies {
				assert.Contains(t, dependencies, expectedDep)
			}
		})
	}
}

func TestGetVariantsAndTasksFromPatchProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	patchedProject := `
buildvariants:
  - name: bv1
    display_name: bv1_display
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task1
        disable: true
      - name: task2
      - name: task3
        depends_on:
          - name: task1
            status: success
tasks:
  - name: task1
  - name: task2
    disable: true
  - name: task3
`

	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, ParserProjectCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, p *patch.Patch, pp *ParserProject){
		"SucceedsWithParserProjectInDB": func(t *testing.T, p *patch.Patch, pp *ParserProject) {
			require.NoError(t, pp.Insert())
			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB

			variantsAndTasks, err := GetVariantsAndTasksFromPatchProject(ctx, env.Settings(), p)
			require.NoError(t, err)
			assert.Len(t, variantsAndTasks.Tasks, 2)
			require.NotNil(t, variantsAndTasks.Variants)
			assert.Len(t, variantsAndTasks.Variants["bv1"].Tasks, 1)
			assert.Equal(t, "task3", variantsAndTasks.Variants["bv1"].Tasks[0].Name)
		},
		"SucceedsWithAlreadyFinalizedPatch": func(t *testing.T, p *patch.Patch, pp *ParserProject) {
			v := Version{
				Id:                   p.Id.Hex(),
				ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
			}
			require.NoError(t, v.Insert())
			require.NoError(t, pp.Insert())
			p.Version = v.Id

			variantsAndTasks, err := GetVariantsAndTasksFromPatchProject(ctx, env.Settings(), p)
			require.NoError(t, err)
			assert.Len(t, variantsAndTasks.Tasks, 2)
			require.NotZero(t, variantsAndTasks)
			assert.Len(t, variantsAndTasks.Variants["bv1"].Tasks, 1)
			assert.Equal(t, "task3", variantsAndTasks.Variants["bv1"].Tasks[0].Name)
		},
		"FailsWithUnfinalizedPatchDoesntHaveParserProjectStorage": func(t *testing.T, p *patch.Patch, pp *ParserProject) {
			_, err := GetVariantsAndTasksFromPatchProject(ctx, env.Settings(), p)
			assert.Error(t, err)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(VersionCollection, ParserProjectCollection))

			project := &Project{}
			pp, err := LoadProjectInto(ctx, []byte(patchedProject), nil, "", project)
			require.NoError(t, err)

			p := &patch.Patch{
				Id: mgobson.NewObjectId(),
			}
			pp.Id = p.Id.Hex()

			tCase(t, p, pp)
		})
	}
}
func (s *projectSuite) TestTagNegation() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const projYml = `
tasks:
  - name: performance-test
    commands:
      - command: shell.exec
        params:
          script: echo "performance test"
  - name: other
    commands:
      - command: shell.exec
        params:
          script: echo "other"
  - name: print
    commands:
      - command: shell.exec
        params:
          script: echo "print"
buildvariants:
  - name: performance-variant
    tags: ["performance"]
    display_name: performance-variant
    run_on:
      - ubuntu1604-small
    tasks:
      - name: performance-test
  - name: other-variant
    tags: ["other"]
    display_name: other-variant
    run_on:
      - ubuntu1604-small
    tasks:
      - name: other
  - name: print-variant
    tags: ["print"]
    display_name: print-variant
    run_on:
      - ubuntu1604-small
    tasks:
      - name: print

patch_aliases:
  - alias: "my alias"
    # Do not run variants tagged with performance or other
    variant_tags: ["!performance !other"]
    task: ".*"
`

	p := &Project{}
	_, err := LoadProjectInto(ctx, []byte(projYml), nil, "", p)
	s.Require().NoError(err)

	pc, err := CreateProjectConfig([]byte(projYml), "")
	s.NoError(err)
	s.NotNil(pc)

	alias := pc.PatchAliases[0]
	pairs, err := p.BuildProjectTVPairsWithAlias([]ProjectAlias{alias}, evergreen.PatchVersionRequester)
	s.NoError(err)
	s.Len(pairs.ExecTasks, 1)
	pairStrs := make([]string, len(pairs.ExecTasks))
	for i, p := range pairs.ExecTasks {
		pairStrs[i] = p.String()
	}
	s.Contains(pairStrs, "print-variant/print")

	for _, pair := range pairs.ExecTasks {
		s.NotEqual("performance-variant", pair.Variant)
		s.NotEqual("other-variant", pair.Variant)
	}
}
