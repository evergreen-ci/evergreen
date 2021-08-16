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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
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

	Convey("When finding a project", t, func() {
		Convey("an error should be thrown if the project ref's identifier is nil", func() {
			projRef := &ProjectRef{
				Id: "",
			}
			version, project, err := FindLatestVersionWithValidProject(projRef.Id)
			So(err, ShouldNotBeNil)
			So(project, ShouldBeNil)
			So(version, ShouldBeNil)
		})

		Convey("if the project file exists and is valid, the project spec within"+
			"should be unmarshalled and returned", func() {
			So(db.ClearCollections(VersionCollection), ShouldBeNil)
			v := &Version{
				Id:         "my_version",
				Owner:      "fakeowner",
				Repo:       "fakerepo",
				Branch:     "fakebranch",
				Identifier: "project_test",
				Requester:  evergreen.RepotrackerVersionRequester,
				Config:     "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
			}
			p := &ProjectRef{
				Id:     "project_test",
				Owner:  "fakeowner",
				Repo:   "fakerepo",
				Branch: "fakebranch",
			}
			require.NoError(t, v.Insert(), "failed to insert test version: %v", v)
			_, _, err := FindLatestVersionWithValidProject(p.Id)
			So(err, ShouldBeNil)

		})
		Convey("if the first version is somehow malformed, return an earlier one", func() {
			So(db.ClearCollections(VersionCollection), ShouldBeNil)
			badVersion := &Version{
				Id:                  "bad_version",
				Owner:               "fakeowner",
				Repo:                "fakerepo",
				Branch:              "fakebranch",
				Identifier:          "project_test",
				Requester:           evergreen.RepotrackerVersionRequester,
				Config:              "this is just nonsense",
				RevisionOrderNumber: 10,
			}
			goodVersion := &Version{
				Id:                  "good_version",
				Owner:               "fakeowner",
				Repo:                "fakerepo",
				Branch:              "fakebranch",
				Identifier:          "project_test",
				Requester:           evergreen.RepotrackerVersionRequester,
				Config:              "owner: fakeowner\nrepo: fakerepo\nbranch: fakebranch",
				RevisionOrderNumber: 8,
			}
			So(badVersion.Insert(), ShouldBeNil)
			So(goodVersion.Insert(), ShouldBeNil)
			v, p, err := FindLatestVersionWithValidProject("project_test")
			So(err, ShouldBeNil)
			So(p, ShouldNotBeNil)
			So(p.Owner, ShouldEqual, "fakeowner")
			So(v.Id, ShouldEqual, "good_version")
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

func TestPopulateExpansions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(VersionCollection, patch.Collection, ProjectRefCollection, task.Collection))
	defer func() {
		assert.NoError(db.ClearCollections(VersionCollection, patch.Collection, ProjectRefCollection, task.Collection))
	}()

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
		Config:              config,
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

	settings := &evergreen.Settings{
		Credentials: map[string]string{"github": "token globalGitHubOauthToken"},
	}
	oauthToken, err := settings.GetGithubOauthToken()
	assert.NoError(err)
	expansions, err := PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 22)
	assert.Equal("0", expansions.Get("execution"))
	assert.Equal("v1", expansions.Get("version_id"))
	assert.Equal("t1", expansions.Get("task_id"))
	assert.Equal("magical task", expansions.Get("task_name"))
	assert.Equal("b1", expansions.Get("build_id"))
	assert.Equal("magic", expansions.Get("build_variant"))
	assert.Equal("0ed7cbd33263043fa95aadb3f6068ef8d076854a", expansions.Get("revision"))
	assert.Equal("mci-favorite", expansions.Get("project"))
	assert.Equal("mci", expansions.Get("project_id"))
	assert.Equal("mci-favorite", expansions.Get("project_identifier"))
	assert.Equal("main", expansions.Get("branch_name"))
	assert.Equal("somebody", expansions.Get("author"))
	assert.Equal("somebody@somewhere.com", expansions.Get("author_email"))
	assert.Equal("d1", expansions.Get("distro_id"))
	assert.Equal("release", expansions.Get("triggered_by_git_tag"))
	assert.Equal("globalGitHubOauthToken", expansions.Get(evergreen.GlobalGitHubTokenExpansion))
	assert.True(expansions.Exists("created_at"))
	assert.Equal("42", expansions.Get("revision_order_id"))
	assert.False(expansions.Exists("is_patch"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.Equal("github_tag", expansions.Get("requester"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.False(expansions.Exists("github_commit"))
	assert.Equal("lie", expansions.Get("cake"))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.PatchVersionRequester},
	}))
	p := patch.Patch{
		Version: v.Id,
	}
	require.NoError(t, p.Insert())

	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 23)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("patch", expansions.Get("requester"))
	assert.False(expansions.Exists("is_commit_queue"))
	assert.False(expansions.Exists("github_repo"))
	assert.False(expansions.Exists("github_author"))
	assert.False(expansions.Exists("github_pr_number"))
	assert.False(expansions.Exists("github_commit"))
	assert.False(expansions.Exists("triggered_by_git_tag"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.MergeTestRequester},
	}))
	p = patch.Patch{
		Version:     v.Id,
		Description: "commit queue message",
	}
	require.NoError(t, p.Insert())
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 25)
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("true", expansions.Get("is_commit_queue"))
	assert.Equal("commit queue message", expansions.Get("commit_message"))
	require.NoError(t, db.ClearCollections(patch.Collection))

	assert.NoError(VersionUpdateOne(bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionRequesterKey: evergreen.GithubPRRequester},
	}))
	p = patch.Patch{
		Version: v.Id,
	}
	require.NoError(t, p.Insert())
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
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

	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 27)
	assert.Equal("github_pr", expansions.Get("requester"))
	assert.Equal("true", expansions.Get("is_patch"))
	assert.Equal("evergreen", expansions.Get("github_repo"))
	assert.Equal("octocat", expansions.Get("github_author"))
	assert.Equal("42", expansions.Get("github_pr_number"))
	assert.Equal("abc123", expansions.Get("github_commit"))
	assert.Equal("wut?", expansions.Get("github_org"))

	upstreamTask := task.Task{
		Id:       "upstreamTask",
		Status:   evergreen.TaskFailed,
		Revision: "abc",
		Project:  "upstreamProject",
	}
	assert.NoError(upstreamTask.Insert())
	upstreamProject := ProjectRef{
		Id:     "upstreamProject",
		Branch: "idk",
	}
	assert.NoError(upstreamProject.Insert())
	taskDoc.TriggerID = "upstreamTask"
	taskDoc.TriggerType = ProjectTriggerLevelTask
	expansions, err = PopulateExpansions(taskDoc, &h, oauthToken)
	assert.NoError(err)
	assert.Len(map[string]string(expansions), 35)
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
	s.Require().NoError(db.ClearCollections(ProjectVarsCollection, ProjectAliasCollection, VersionCollection, build.Collection, task.Collection))
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
	pairs, displayTaskPairs, err := s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[0]})
	s.NoError(err)
	s.Len(pairs, 11)
	pairStrs := make([]string, len(pairs))
	for i, p := range pairs {
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
	s.Contains(pairStrs, "bv_3/disabled_task")
	s.Require().Len(displayTaskPairs, 1)
	s.Equal("bv_1/memes", displayTaskPairs[0].String())

	// test that the .*_2 regex on variants selects just bv_2
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[1]})
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Equal("bv_2", pair.Variant)
	}
	s.Empty(displayTaskPairs)

	// test that the .*_2 regex on tasks selects just the _2 tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[2]})
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Contains(pair.TaskName, "task_2")
	}
	s.Empty(displayTaskPairs)

	// test that the 'a' tag only selects 'a' tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[3]})
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.Contains(pair.TaskName, "a_task")
	}
	s.Empty(displayTaskPairs)

	// test that the .*_2 regex selects the union of both
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[4]})
	s.NoError(err)
	s.Len(pairs, 4)
	for _, pair := range pairs {
		s.NotEqual("b_task_1", pair.TaskName)
	}
	s.Empty(displayTaskPairs)

	// test for display tasks
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[5]})
	s.NoError(err)
	s.Empty(pairs)
	s.Require().Len(displayTaskPairs, 1)
	s.Equal("bv_1/memes", displayTaskPairs[0].String())

	// test for alias including a task belong to a disabled variant
	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[6]})
	s.NoError(err)
	s.Require().Len(pairs, 1)
	s.Equal("bv_3/disabled_task", pairs[0].String())
	s.Empty(displayTaskPairs)

	pairs, displayTaskPairs, err = s.project.BuildProjectTVPairsWithAlias([]ProjectAlias{s.aliases[8]})
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
	s.Len(patchDoc.Tasks, 6)

	// test all tasks expansion with named buildvariant expands unnamed buildvariant
	patchDoc.BuildVariants = []string{"bv_1"}
	patchDoc.Tasks = []string{"all"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 6)

	// test all variants expansion with named task
	patchDoc.BuildVariants = []string{"all"}
	patchDoc.VariantsTasks = []patch.VariantTasks{}

	s.project.BuildProjectTVPairs(&patchDoc, "")

	s.Len(patchDoc.BuildVariants, 2)
	s.Len(patchDoc.Tasks, 6)
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
			s.Empty(vt.DisplayTasks[0].ExecTasks)
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

	config := NewPatchTaskIdTable(p, v, pairs, "project_identifier")
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
	intermediate, err := createIntermediateProject([]byte(projYml))
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

func TestModuleList(t *testing.T) {
	assert := assert.New(t)

	projModules := ModuleList{
		{Name: "enterprise", Repo: "git@github.com:something/enterprise.git", Branch: "main"},
		{Name: "wt", Repo: "git@github.com:else/wt.git", Branch: "develop"},
	}

	manifest1 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.True(projModules.IsIdentical(manifest1))

	manifest2 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "different branch", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
		},
	}
	assert.False(projModules.IsIdentical(manifest2))

	manifest3 := manifest.Manifest{
		Modules: map[string]*manifest.Module{
			"wt":         &manifest.Module{Branch: "develop", Repo: "wt", Owner: "else", Revision: "123"},
			"enterprise": &manifest.Module{Branch: "main", Repo: "enterprise", Owner: "something", Revision: "abc"},
			"extra":      &manifest.Module{Branch: "main", Repo: "repo", Owner: "something", Revision: "abc"},
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

func TestLoggerMerge(t *testing.T) {
	assert := assert.New(t)

	var config1 *LoggerConfig
	config2 := &LoggerConfig{
		Agent:  []LogOpts{{Type: LogkeeperLogSender}},
		System: []LogOpts{{Type: LogkeeperLogSender}},
		Task:   []LogOpts{{Type: LogkeeperLogSender}},
	}

	assert.Nil(mergeAllLogs(config1, config1))

	merged := mergeAllLogs(config1, config2)
	assert.NotNil(merged)
	assert.Equal(len(merged.Agent), 1)
	assert.Equal(len(merged.System), 1)
	assert.Equal(len(merged.Task), 1)

	merged = mergeAllLogs(config2, config1)
	assert.NotNil(merged)
	assert.Equal(len(merged.Agent), 1)
	assert.Equal(len(merged.System), 1)
	assert.Equal(len(merged.Task), 1)

	config1 = &LoggerConfig{
		Agent:  []LogOpts{{LogDirectory: "a"}},
		System: []LogOpts{{LogDirectory: "a"}},
		Task:   []LogOpts{{LogDirectory: "a"}},
	}

	merged = mergeAllLogs(config2, config1)
	assert.NotNil(merged)
	assert.Equal(len(merged.Agent), 2)
	assert.Equal(len(merged.System), 2)
	assert.Equal(len(merged.Task), 2)
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
				"function": &YAMLCommandSet{
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
				"function": &YAMLCommandSet{
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
							{Name: "t1"},
							{Name: "t2"},
						},
					}, {
						Name: "bv2",
						Tasks: []BuildVariantTaskUnit{
							{Name: "t2"},
							{Name: "t3"},
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
					{Name: "t0"},
					{Name: "t1", DependsOn: []TaskUnitDependency{{Name: "t0", Variant: "bv0"}}}},
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

func TestSkipOnPatch(t *testing.T) {
	falseTmp := false
	bvt := BuildVariantTaskUnit{Patchable: &falseTmp}

	b := &build.Build{Requester: evergreen.RepotrackerVersionRequester}
	assert.False(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())
	assert.False(t, bvt.SkipOnRequester(b.Requester))

	b.Requester = evergreen.PatchVersionRequester
	assert.True(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())
	assert.True(t, bvt.SkipOnRequester(b.Requester))

	b.Requester = evergreen.GithubPRRequester
	assert.True(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())
	assert.True(t, bvt.SkipOnRequester(b.Requester))

	b.Requester = evergreen.MergeTestRequester
	assert.True(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())
	assert.True(t, bvt.SkipOnRequester(b.Requester))

	bvt.Patchable = nil
	assert.False(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())
	assert.False(t, bvt.SkipOnRequester(b.Requester))
}

func TestSkipOnNonPatch(t *testing.T) {
	boolTmp := true
	bvt := BuildVariantTaskUnit{PatchOnly: &boolTmp}
	b := &build.Build{Requester: evergreen.RepotrackerVersionRequester}
	assert.True(t, !b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())
	assert.True(t, bvt.SkipOnRequester(b.Requester))

	b.Requester = evergreen.PatchVersionRequester
	assert.False(t, !b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())
	assert.False(t, bvt.SkipOnRequester(b.Requester))

	b.Requester = evergreen.GithubPRRequester
	assert.False(t, !b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())
	assert.False(t, bvt.SkipOnRequester(b.Requester))

	bvt.PatchOnly = nil
	assert.False(t, !b.IsPatchBuild() && bvt.SkipOnNonPatchBuild())
	assert.False(t, b.IsPatchBuild() && bvt.SkipOnPatchBuild())

	assert.False(t, bvt.SkipOnRequester(b.Requester))
}

func TestSkipOnGitTagBuild(t *testing.T) {
	bvt := BuildVariantTaskUnit{AllowForGitTag: boolPtr(false)}
	r := evergreen.GitTagRequester
	assert.True(t, evergreen.IsGitTagRequester(r) && bvt.SkipOnGitTagBuild())
	assert.False(t, evergreen.IsGitTagRequester(r) && !bvt.SkipOnGitTagBuild())
	assert.True(t, bvt.SkipOnRequester(r))

	r = evergreen.PatchVersionRequester
	assert.False(t, evergreen.IsGitTagRequester(r) && bvt.SkipOnGitTagBuild())
	assert.False(t, bvt.SkipOnRequester(r))

	r = evergreen.GithubPRRequester
	assert.False(t, evergreen.IsGitTagRequester(r) && bvt.SkipOnGitTagBuild())
	assert.False(t, bvt.SkipOnRequester(r))

	bvt.AllowForGitTag = nil
	r = evergreen.GitTagRequester
	assert.False(t, evergreen.IsGitTagRequester(r) && bvt.SkipOnGitTagBuild())
	assert.False(t, bvt.SkipOnRequester(r))
}

func TestSkipOnNonGitTagBuild(t *testing.T) {
	bvt := BuildVariantTaskUnit{GitTagOnly: boolPtr(true)}
	r := evergreen.GitTagRequester
	assert.False(t, !evergreen.IsGitTagRequester(r) && bvt.SkipOnNonGitTagBuild())
	assert.False(t, !evergreen.IsGitTagRequester(r) && !bvt.SkipOnNonGitTagBuild())
	assert.False(t, bvt.SkipOnRequester(r))

	r = evergreen.PatchVersionRequester
	assert.True(t, !evergreen.IsGitTagRequester(r) && bvt.SkipOnNonGitTagBuild())
	assert.True(t, bvt.SkipOnRequester(r))

	r = evergreen.GithubPRRequester
	assert.True(t, !evergreen.IsGitTagRequester(r) && bvt.SkipOnNonGitTagBuild())
	assert.True(t, bvt.SkipOnRequester(r))

	bvt.GitTagOnly = nil
	assert.False(t, !evergreen.IsGitTagRequester(r) && bvt.SkipOnNonGitTagBuild())
	assert.False(t, bvt.SkipOnRequester(r))
}
