package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	yaml "gopkg.in/yaml.v2"
)

func TestFindProject(t *testing.T) {

	Convey("When finding a project", t, func() {
		Convey("an error should be thrown if the project ref's identifier is nil", func() {
			projRef := &ProjectRef{
				Identifier: "",
			}
			project, err := FindLastKnownGoodProject(projRef.Identifier)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
		})

		Convey("if the project file exists and is valid, the project spec within"+
			"should be unmarshalled and returned", func() {
			v := &Version{
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
				Config:     "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
			}
			p := &ProjectRef{
				Identifier: "project_test",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
			}
			require.NoError(t, v.Insert(), "failed to insert test version: %v", v)
			_, err := FindLastKnownGoodProject(p.Identifier)
			So(err, ShouldBeNil)

		})

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

func TestGetModuleRepoName(t *testing.T) {

	Convey("With a module", t, func() {

		Convey("getting the repo owner and name should return the repo"+
			" field, split at the ':' and removing the .git from"+
			" the end", func() {

			module := &Module{
				Repo: "blecch:owner/repo.git",
			}

			owner, name := module.GetRepoOwnerAndName()
			So(owner, ShouldEqual, "owner")
			So(name, ShouldEqual, "repo")

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
					Stepback:        boolPtr(false),
					DependsOn:       []TaskUnitDependency{{Name: "other"}},
					Priority:        1000,
					Patchable:       boolPtr(false),
				},
			},
			BuildVariants: []BuildVariant{
				{
					Name:  "test",
					Tasks: []BuildVariantTaskUnit{{Name: "task1", Priority: 5}},
				},
			},
		}

		Convey("updating a BuildVariantTaskUnit with unset fields", func() {
			bvt := project.BuildVariants[0].Tasks[0]
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should inherit the unset fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 500)
				So(bvt.Stepback, ShouldNotBeNil)
				So(bvt.Patchable, ShouldNotBeNil)
				So(len(bvt.DependsOn), ShouldEqual, 1)

				Convey("but not set fields", func() { So(bvt.Priority, ShouldEqual, 5) })
			})
		})

		Convey("updating a BuildVariantTaskUnit with set fields", func() {
			bvt := BuildVariantTaskUnit{
				Name:            "task1",
				ExecTimeoutSecs: 2,
				Stepback:        boolPtr(true),
				DependsOn:       []TaskUnitDependency{{Name: "task2"}, {Name: "task3"}},
			}
			spec := project.GetSpecForTask("task1")
			So(spec.Name, ShouldEqual, "task1")
			bvt.Populate(spec)

			Convey("should not inherit set fields from the Project", func() {
				So(bvt.Name, ShouldEqual, "task1")
				So(bvt.ExecTimeoutSecs, ShouldEqual, 2)
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

func boolPtr(b bool) *bool {
	return &b
}

func TestGetTaskGroup(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(VersionCollection), "failed to clear collections")
	tgName := "example_task_group"
	projYml := `
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- name: example_task_group
  max_hosts: 2
  setup_group:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_group:
  - command: shell.exec
    params:
      script: "echo teardown_group"
  setup_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  tasks:
  - example_task_1
  - example_task_2
`
	proj, errs := projectFromYAML([]byte(projYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	v := Version{
		Id:     "v1",
		Config: projYml,
	}
	t1 := task.Task{
		Id:        "t1",
		TaskGroup: tgName,
		Version:   v.Id,
	}

	tg, err := GetTaskGroup(tgName, &TaskConfig{
		Version: &v,
		Task:    &t1,
	})
	assert.NoError(err)
	assert.Equal(tgName, tg.Name)
	assert.Len(tg.Tasks, 2)
	assert.Equal(2, tg.MaxHosts)
}

func TestPopulateExpansions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(VersionCollection, patch.Collection, ProjectRefCollection, task.Collection))

	h := host.Host{
		Id: "h",
		Distro: distro.Distro{
			Id:      "d1",
			WorkDir: "/home/evg",
			Expansions: []distro.Expansion{
				distro.Expansion{
					Key:   "note",
					Value: "huge success",
				},
				distro.Expansion{
					Key:   "cake",
					Value: "truth",
				},
			},
		},
	}
	config := `
buildvariants:
- name: magic
  expansions:
    cake: lie
    github_org: wut?
`
	v := &Version{
		Id:                  "v1",
		Branch:              "master",
		Author:              "somebody",
		RevisionOrderNumber: 42,
		Config:              config,
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

	settings := &evergreen.Settings{
		Credentials: map[string]string{"github": "token globalGitHubOauthToken"},
	}
	oauthToken, err := settings.GetGithubOauthToken()
	assert.NoError(err)
	expansions, err := PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 17)
	assert.Equal("0", expansions.Get("execution"))
	assert.Equal("v1", expansions.Get("version_id"))
	assert.Equal("t1", expansions.Get("task_id"))
	assert.Equal("magical task", expansions.Get("task_name"))
	assert.Equal("b1", expansions.Get("build_id"))
	assert.Equal("magic", expansions.Get("build_variant"))
	assert.Equal("0ed7cbd33263043fa95aadb3f6068ef8d076854a", expansions.Get("revision"))
	assert.Equal("mci", expansions.Get("project"))
	assert.Equal("master", expansions.Get("branch_name"))
	assert.Equal("somebody", expansions.Get("author"))
	assert.Equal("d1", expansions.Get("distro_id"))
	assert.Equal("globalGitHubOauthToken", expansions.Get(evergreen.GlobalGitHubTokenExpansion))
	assert.True(expansions.Exists("created_at"))
	assert.Equal("42", expansions.Get("revision_order_id"))
	assert.False(expansions.Exists("is_patch"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.Equal("lie", expansions.Get("cake"))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.PatchVersionRequester},
	}))

	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 18)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.MergeTestRequester},
	}))
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 19)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("true", expansions.Get("is_commit_queue"))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.GithubPRRequester},
	}))
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 18)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))

	patchDoc := &patch.Patch{
		Version: v.Id,
		GithubPatchData: patch.GithubPatch{
			PRNumber:  42,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			Author:    "octocat",
		},
	}
	assert.NoError(patchDoc.Insert())

	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 21)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("evergreen", expansions.Get("github_repo"))
	assert.Equal("octocat", expansions.Get("github_author"))
	assert.Equal("42", expansions.Get("github_pr_number"))
	assert.Equal("wut?", expansions.Get("github_org"))

	upstreamTask := task.Task{
		Id:       "upstreamTask",
		Status:   evergreen.TaskFailed,
		Revision: "abc",
		Project:  "upstreamProject",
	}
	assert.NoError(upstreamTask.Insert())
	upstreamProject := ProjectRef{
		Identifier: "upstreamProject",
		Branch:     "idk",
	}
	assert.NoError(upstreamProject.Insert())
	taskDoc.TriggerID = "upstreamTask"
	taskDoc.TriggerType = ProjectTriggerLevelTask
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 29)
	assert.Equal(taskDoc.TriggerID, expansions.Get("trigger_event_identifier"))
	assert.Equal(taskDoc.TriggerType, expansions.Get("trigger_event_type"))
	assert.Equal(upstreamTask.Revision, expansions.Get("trigger_revision"))
	assert.Equal(upstreamTask.Status, expansions.Get("trigger_status"))
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
	s.Require().NoError(db.ClearCollections(ProjectVarsCollection, ProjectAliasCollection, VersionCollection, build.Collection))
	s.vars = ProjectVars{
		Id: "project",
	}
	s.Require().NoError(s.vars.Insert())

	s.aliases = []ProjectAlias{
		{
			ProjectID: "project",
			Alias:     "all",
			Variant:   ".*",
			Task:      ".*",
		},
		{
			ProjectID: "project",
			Alias:     "bv2",
			Variant:   ".*_2",
			Task:      ".*",
		},
		{
			ProjectID: "project",
			Alias:     "2tasks",
			Variant:   ".*",
			Task:      ".*_2",
		},
		{
			ProjectID: "project",
			Alias:     "aTags",
			Variant:   ".*",
			TaskTags:  []string{"a"},
		},
		{
			ProjectID: "project",
			Alias:     "2tasks(obsolete)", // Remains from times when tags and tasks were allowed together
			Variant:   ".*",
			Task:      ".*_2",
		},
		{
			ProjectID: "project",
			Alias:     "memes",
			Variant:   ".*",
			Task:      "memes",
		},
		{
			ProjectID: "project",
			Alias:     "disabled_stuff",
			Variant:   "bv_3",
			Task:      "disabled_.*",
		},
		{
			ProjectID: "project",
			Alias:     "part_of_memes",
			Variant:   "bv_1",
			TaskTags:  []string{"part_of_memes"},
		},
		{
			ProjectID:   "project",
			Alias:       "even_bvs",
			VariantTags: []string{"even"},
			TaskTags:    []string{"a"},
		},
	}
	for _, alias := range s.aliases {
		s.NoError(alias.Upsert())
	}

	s.project = &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			{
				Name: "bv_1",
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "a_task_1",
					},
					{
						Name: "a_task_2",
					},
					{
						Name: "b_task_1",
					},
					{
						Name: "b_task_2",
					},
					{
						Name: "wow_task",
						Requires: []TaskUnitRequirement{
							{
								Name:    "a_task_1",
								Variant: "bv_1",
							},
							{
								Name:    "a_task_1",
								Variant: "bv_2",
							},
						},
					},
					{
						Name: "9001_task",
						DependsOn: []TaskUnitDependency{
							{
								Name:    "a_task_2",
								Variant: "bv_2",
							},
						},
					},
					{
						Name: "very_task",
					},
					{
						Name:      "another_disabled_task",
						Patchable: boolPtr(false),
					},
				},
				DisplayTasks: []DisplayTask{
					{
						Name:           "memes",
						ExecutionTasks: []string{"wow_task", "9001_task", "very_task", "another_disabled_task"},
					},
				},
			},
			{
				Name: "bv_2",
				Tags: []string{"even"},
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "a_task_1",
					},
					{
						Name: "a_task_2",
					},
					{
						Name: "b_task_1",
					},
					{
						Name: "b_task_2",
					},
					{
						Name:      "another_disabled_task",
						Patchable: boolPtr(false),
					},
				},
			},
			{
				Name:     "bv_3",
				Disabled: true,
				Tasks: []BuildVariantTaskUnit{
					{
						Name: "disabled_task",
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
				Name: "wow_task",
				Tags: []string{"part_of_memes"},
			},
			{
				Name: "9001_task",
			},
			{
				Name: "very_task",
			},
			{
				Name: "disabled_task",
			},
			{
				Name:      "another_disabled_task",
				Patchable: boolPtr(false),
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"go generate a thing": &YAMLCommandSet{
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
	pairs, displayTaskPairs, err := s.project.BuildProjectTVPairsWithAlias(s.aliases[0].Alias)
	s.NoError(err)
	s.Len(pairs, 12)
	pairStrs := make([]string, len(pairs))
	for i, p := range pairs {
		pairStrs[i] = p.String()
	}
	s.Contains(pairStrs, "bv_1/a_task_1")
	s.Contains(pairStrs, "bv_1/a_task_2")
	s.Contains(pairStrs, "bv_1/b_task_1")
	s.Contains(pairStrs, "bv_1/b_task_2")
	s.Contains(pairStrs, "bv_1/wow_task")
	s.Contains(pairStrs, "bv_1/9001_task")
	s.Contains(pairStrs, "bv_1/very_task")
	s.Contains(pairStrs, "bv_2/a_task_1")
	s.Contains(pairStrs, "bv_2/a_task_2")
	s.Contains(pairStrs, "bv_2/b_task_1")
	s.Contains(pairStrs, "bv_2/b_task_2")
	s.Contains(pairStrs, "bv_3/disabled_task")
	s.Require().Len(displayTaskPairs, 1)
	s.Equal("bv_1/memes", displayTaskPairs[0].String())

	// test that the .*_2 regex on variants selects just bv_2
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[1].Alias)
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Equal("bv_2", pair.Variant)
	}
	s.Empty(displayTaskPairs)

	// test that the .*_2 regex on tasks selects just the _2 tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[2].Alias)
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Contains(pair.TaskName, "task_2")
	}
	s.Empty(displayTaskPairs)

	// test that the 'a' tag only selects 'a' tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[3].Alias)
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Contains(pair.TaskName, "a_task")
	}
	s.Empty(displayTaskPairs)

	// test that the .*_2 regex selects the union of both
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[4].Alias)
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.NotEqual("b_task_1", pair.TaskName)
	}
	s.Empty(displayTaskPairs)

	// test for display tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[5].Alias)
	s.NoError(err)
	s.Empty(pairs)
	s.Require().Len(displayTaskPairs, 1)
	s.Equal("bv_1/memes", displayTaskPairs[0].String())

	// test for alias including a task belong to a disabled variant
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[6].Alias)
	s.NoError(err)
	s.Require().Len(pairs, 1)
	s.Equal("bv_3/disabled_task", pairs[0].String())
	s.Empty(displayTaskPairs)

	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias(s.aliases[8].Alias)
	s.NoError(err)
	s.Require().Len(pairs, 2)
	s.Equal("bv_2/a_task_1", pairs[0].String())
	s.Equal("bv_2/a_task_2", pairs[1].String())
	s.Empty(displayTaskPairs)
}

func (s *projectSuite) TestBuildProjectTVPairs() {
	// test all expansions
	patchDoc := patch.Patch{
		BuildVariants: []string{"all"},
		Tasks:         []string{"all"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 7)

	// test all tasks expansion with named buildvariant expands unnamed buildvariant
	patchDoc.BuildVariants = []string{"bv_1"}
	patchDoc.Tasks = []string{"all"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 7)

	// test all variants expansion with named task
	patchDoc.BuildVariants = []string{"all"}
	patchDoc.Tasks = []string{"wow_task"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 5)
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
	s.Len(patchDoc.Tasks, 5)
	s.Contains(patchDoc.Tasks, "very_task")
	s.Contains(patchDoc.Tasks, "wow_task")
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Contains(patchDoc.Tasks, "9001_task")
	s.Contains(patchDoc.Tasks, "a_task_2")
	s.Require().Len(patchDoc.VariantsTasks, 2)
	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Contains(vt.Tasks, "very_task")
			s.Contains(vt.Tasks, "wow_task")
			s.Contains(vt.Tasks, "a_task_1")
			s.Contains(vt.Tasks, "9001_task")
			s.Require().Len(vt.DisplayTasks, 1)
			s.Equal("memes", vt.DisplayTasks[0].Name)

		} else if vt.Variant == "bv_2" {
			s.Contains(vt.Tasks, "a_task_1")
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
	s.Equal([]string{"bv_3"}, patchDoc.BuildVariants)
	s.Equal([]string{"disabled_task"}, patchDoc.Tasks)
	s.Require().Len(patchDoc.VariantsTasks, 1)
	s.Equal("bv_3", patchDoc.VariantsTasks[0].Variant)
	s.Equal([]string{"disabled_task"}, patchDoc.VariantsTasks[0].Tasks)
	s.Empty(patchDoc.VariantsTasks[0].DisplayTasks)

	patchDoc = patch.Patch{
		BuildVariants: []string{"bv_3"},
		Tasks:         []string{"disabled_task"},
	}

	s.project.BuildProjectTVPairs(&patchDoc, "")
	s.Equal([]string{"bv_3"}, patchDoc.BuildVariants)
	s.Equal([]string{"disabled_task"}, patchDoc.Tasks)
	s.Require().Len(patchDoc.VariantsTasks, 1)
	s.Equal("bv_3", patchDoc.VariantsTasks[0].Variant)
	s.Equal([]string{"disabled_task"}, patchDoc.VariantsTasks[0].Tasks)
	s.Empty(patchDoc.VariantsTasks[0].DisplayTasks)
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
	s.Len(patchDoc.Tasks, 5)
	s.Contains(patchDoc.Tasks, "wow_task")
	s.Contains(patchDoc.Tasks, "9001_task")
	s.Contains(patchDoc.Tasks, "very_task")
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Contains(patchDoc.Tasks, "a_task_2")
	s.Require().Len(patchDoc.VariantsTasks, 2)

	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Require().Len(vt.Tasks, 4)
			s.Contains(vt.Tasks, "a_task_1")
			s.Contains(vt.Tasks, "very_task")
			s.Contains(vt.Tasks, "9001_task")
			s.Contains(vt.Tasks, "wow_task")
			s.Require().Len(vt.DisplayTasks, 1)
			s.Equal("memes", vt.DisplayTasks[0].Name)
			s.Empty(vt.DisplayTasks[0].ExecTasks)

		} else if vt.Variant == "bv_2" {
			s.Require().Len(vt.Tasks, 2)
			s.Contains(vt.Tasks, "a_task_1")
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
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "wow_task")
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Require().Len(patchDoc.VariantsTasks, 2)
	for _, vt := range patchDoc.VariantsTasks {
		if vt.Variant == "bv_1" {
			s.Require().Len(vt.Tasks, 2)
			s.Contains(vt.Tasks, "a_task_1")
			s.Contains(vt.Tasks, "wow_task")
			s.Empty(vt.DisplayTasks, 1)

		} else if vt.Variant == "bv_2" {
			s.Require().Len(vt.Tasks, 1)
			s.Contains(vt.Tasks, "a_task_1")
			s.Empty(vt.DisplayTasks)

		} else {
			s.T().Fail()
		}
		s.Empty(vt.DisplayTasks)
	}
}

func (s *projectSuite) TestBuildProjectTVPairsWithExecutionTask() {
	patchDoc := patch.Patch{
		BuildVariants: []string{"bv_1"},
		Tasks:         []string{"wow_task"},
	}
	s.project.BuildProjectTVPairs(&patchDoc, "")
	s.Len(patchDoc.BuildVariants, 2)
	s.Contains(patchDoc.BuildVariants, "bv_1")
	s.Contains(patchDoc.BuildVariants, "bv_2")
	s.Len(patchDoc.Tasks, 5)
	s.Contains(patchDoc.Tasks, "wow_task")
	s.Contains(patchDoc.Tasks, "a_task_1")
	s.Require().Len(patchDoc.VariantsTasks, 2)
}

func (s *projectSuite) TestNewPatchTaskIdTable() {
	p := &Project{
		Identifier: "project_identifier",
		Tasks: []ProjectTask{
			ProjectTask{
				Name: "task1",
			},
			ProjectTask{
				Name: "task2",
			},
			ProjectTask{
				Name: "task3",
			},
		},
		BuildVariants: []BuildVariant{
			BuildVariant{
				Name:  "test",
				Tasks: []BuildVariantTaskUnit{{Name: "group_1"}},
			},
		},
		TaskGroups: []TaskGroup{
			TaskGroup{
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

	config := NewPatchTaskIdTable(p, v, pairs)
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
	intermediate, errs := createIntermediateProject([]byte(projYml))
	s.Len(errs, 0)
	marshaled, err := yaml.Marshal(intermediate)
	s.NoError(err)
	unmarshaled := parserProject{}
	s.NoError(yaml.Unmarshal(marshaled, &unmarshaled))
}

func (s *projectSuite) TestFetchVersionsAndAssociatedBuilds() {
	v1 := Version{
		Id:                  "v1",
		Identifier:          s.project.Identifier,
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now(),
		RevisionOrderNumber: 1,
	}
	s.NoError(v1.Insert())
	v2 := Version{
		Id:                  "v2",
		Identifier:          s.project.Identifier,
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now().Add(1 * time.Minute),
		RevisionOrderNumber: 2,
	}
	s.NoError(v2.Insert())
	v3 := Version{
		Id:                  "v3",
		Identifier:          s.project.Identifier,
		Requester:           evergreen.RepotrackerVersionRequester,
		CreateTime:          time.Now().Add(5 * time.Minute),
		RevisionOrderNumber: 3,
	}
	s.NoError(v3.Insert())
	b1 := build.Build{
		Id:      "b1",
		Version: v1.Id,
	}
	s.NoError(b1.Insert())
	b2 := build.Build{
		Id:      "b2",
		Version: v2.Id,
	}
	s.NoError(b2.Insert())
	b3 := build.Build{
		Id:      "b3",
		Version: v3.Id,
	}
	s.NoError(b3.Insert())

	versions, builds, err := FetchVersionsAndAssociatedBuilds(s.project, 0, 10, false)
	s.NoError(err)
	s.Equal(v3.Id, versions[0].Id)
	s.Equal(v2.Id, versions[1].Id)
	s.Equal(v1.Id, versions[2].Id)
	s.Equal(b1.Id, builds[v1.Id][0].Id)
	s.Equal(b2.Id, builds[v2.Id][0].Id)
	s.Equal(b3.Id, builds[v3.Id][0].Id)
}

func (s *projectSuite) TestIsGenerateTask() {
	s.False(s.project.IsGenerateTask("a_task_1"))
	s.True(s.project.IsGenerateTask("a_task_2"))
	s.True(s.project.IsGenerateTask("b_task_1"))
	s.False(s.project.IsGenerateTask("b_task_2"))
	s.False(s.project.IsGenerateTask("wow_task"))
	s.False(s.project.IsGenerateTask("9001_task"))
	s.False(s.project.IsGenerateTask("very_task"))
	s.False(s.project.IsGenerateTask("disabled_task"))
	s.False(s.project.IsGenerateTask("another_disabled_task"))
	s.False(s.project.IsGenerateTask("task_does_not_exist"))
}

func TestModuleList(t *testing.T) {
	assert := assert.New(t)

	projModules := ModuleList{
		{Name: "enterprise", Repo: "git@github.com:something/enterprise.git", Branch: "master"},
		{Name: "wt", Repo: "git@github.com:else/wt.git", Branch: "develop"},
	}

	manifest1 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.True(projModules.IsIdentical(manifest1))

	manifest2 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "different branch", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(projModules.IsIdentical(manifest2))

	manifest3 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "master", Repo: "enterprise", Owner: "something", Revision: "abc"},
			"extra":      &manifest.Module{Branch: "master", Repo: "repo", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(projModules.IsIdentical(manifest3))

	manifest4 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt": &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
		},
	}
	assert.False(projModules.IsIdentical(manifest4))
}

func TestLoggerConfigValidate(t *testing.T) {
	assert := assert.New(t)

	var config *LoggerConfig
	assert.NotPanics(func() {
		assert.NoError(config.IsValid())
	})
	config = &LoggerConfig{}
	assert.NoError(config.IsValid())

	config = &LoggerConfig{
		Agent: []LogOpts{{Type: "foo"}},
	}
	assert.EqualError(config.IsValid(), "invalid agent logger config: foo is not a valid log sender")

	config = &LoggerConfig{
		System: []LogOpts{{Type: SplunkLogSender}},
	}
	assert.EqualError(config.IsValid(), "invalid system logger config: Splunk logger requires a server URL\nSplunk logger requires a token")
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
