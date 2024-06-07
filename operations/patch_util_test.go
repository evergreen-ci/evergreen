package operations

import (
	"context"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PatchUtilTestSuite struct {
	suite.Suite
	tempDir        string
	testConfigFile string
}

func TestPatchUtilTestSuite(t *testing.T) {
	suite.Run(t, new(PatchUtilTestSuite))
}

func (s *PatchUtilTestSuite) SetupSuite() {
	dir := s.T().TempDir()

	s.tempDir = dir
	s.testConfigFile = dir + ".evergreen.yml"
}

func (s *PatchUtilTestSuite) TestLoadAliasFromFile() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  alias: testing
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Equal("testing", pp.Alias)
	s.Nil(pp.Variants)
	s.Nil(pp.Tasks)
}

func (s *PatchUtilTestSuite) TestLoadVariantsTasksFromFile() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Zero(pp.Alias)
	s.Contains(pp.Variants, "myvariant1")
	s.Contains(pp.Variants, "myvariant2")
	s.Contains(pp.Tasks, "mytask1")
	s.Contains(pp.Tasks, "mytask2")
}

func (s *PatchUtilTestSuite) TestAliasFromCLI() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  variants:
   - myvariant1
   - myvariant2
  tasks:
   - mytask1
   - mytask2`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Alias:       "testing",
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Equal("testing", pp.Alias)
	s.Nil(pp.Variants)
	s.Nil(pp.Tasks)
}

func (s *PatchUtilTestSuite) TestVariantsTasksFromCLI() {
	// Set up the user config file
	fileContents := `projects:
- name: mci
  default: true
  alias: testing`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	pp := patchParams{
		Project:     "mci",
		Variants:    []string{"myvariant1", "myvariant2"},
		Tasks:       []string{"mytask1", "mytask2"},
		SkipConfirm: true,
	}
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	s.Require().NoError(pp.loadAlias(conf))
	s.Require().NoError(pp.loadVariants(conf))
	s.Require().NoError(pp.loadTasks(conf))

	s.Zero(pp.Alias)
	s.Contains(pp.Variants, "myvariant1")
	s.Contains(pp.Variants, "myvariant2")
	s.Contains(pp.Tasks, "mytask1")
	s.Contains(pp.Tasks, "mytask2")
}

func (s *PatchUtilTestSuite) TestParseGitVersionString() {
	versionStrings := map[string]string{
		"git version 2.19.1":                   "2.19.1",
		"git version 2.24.3 (Apple Git-128)":   "2.24.3",
		"git version 2.21.1 (Apple Git-122.3)": "2.21.1",
		"git version 2.16.2.windows.1":         "2.16.2.windows.1",
	}

	for versionString, version := range versionStrings {
		parsedVersion, err := parseGitVersion(versionString)
		s.NoError(err)
		s.Equal(version, parsedVersion)
	}
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsLoadsExplicitAlias() {
	pp := patchParams{
		Project: "project",
		Alias:   "duck",
	}
	conf := &ClientSettings{
		Projects: []ClientProjectConf{
			{
				Name:     "project",
				Variants: []string{"default-bv0", "default-bv1"},
				Tasks:    []string{"default-task0", "default-task1"},
			},
		},
	}

	pp.setNonRepeatedDefaults(conf)

	s.Equal("duck", pp.Alias, "alias should not be defaulted")
	s.Empty(pp.Variants, "variants should not be defaulted for explicit alias")
	s.Empty(pp.Tasks, "tasks should not be defaulted for explicit alias")
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsWithLocalAliasOverridesOtherDefaults() {
	pp := patchParams{
		Project: "project",
		Alias:   "duck",
	}
	conf := &ClientSettings{
		Projects: []ClientProjectConf{
			{
				Name:     "project",
				Alias:    "chicken",
				Variants: []string{"default-bv0", "default-bv1"},
				Tasks:    []string{"default-task0", "default-task1"},
				LocalAliases: []model.ProjectAlias{
					{
						Alias: "duck",
					},
				},
			},
		},
	}

	pp.setNonRepeatedDefaults(conf)

	s.Equal("duck", pp.Alias, "should use local alias instead of default project alias")
	s.Empty(pp.Variants, "variants should not be defaulted for explicit local alias")
	s.Empty(pp.Tasks, "tasks should not be defaulted for explicit local alias")
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsLoadsDefaultAlias() {
	pp := patchParams{
		Project: "project",
	}
	conf := &ClientSettings{
		Projects: []ClientProjectConf{
			{
				Name:     "project",
				Alias:    "orange",
				Variants: []string{"default-bv0", "default-bv1"},
				Tasks:    []string{"default-task0", "default-task1"},
			},
		},
	}

	pp.setNonRepeatedDefaults(conf)

	s.Equal("orange", pp.Alias, "alias should be defaulted if none is specified")
	s.Empty(pp.Variants, "variants should not be defaulted when using default alias")
	s.Empty(pp.Tasks, "tasks should not be defaulted when using default alias")
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsLoadsDefaultVariantsAndTasksWithoutAlias() {
	pp := patchParams{
		Project: "project",
	}
	conf := &ClientSettings{
		Projects: []ClientProjectConf{
			{
				Name:     "project",
				Variants: []string{"default-bv0", "default-bv1"},
				Tasks:    []string{"default-task0", "default-task1"},
			},
		},
	}

	pp.setNonRepeatedDefaults(conf)

	s.Empty(pp.Alias, "should not set an alias when there is no default")
	s.ElementsMatch([]string{"default-bv0", "default-bv1"}, pp.Variants, "variants should be defaulted")
	s.ElementsMatch([]string{"default-task0", "default-task1"}, pp.Tasks, "tasks should be defaulted")
}

func (s *PatchUtilTestSuite) TestGetRemoteFromOutput() {
	out := `
	origin  git@github.com:ZackarySantana/evergreen.git (fetch)
	origin  git@github.com:ZackarySantana/evergreen.git (push)
	upstream        https://github.com/evergreen-ci/evergreen (fetch)
	upstream        https://github.com/evergreen-ci/evergreen (push)
	`

	repo, err := getRemoteFromOutput(out, "ZackarySantana", "evergreen")
	s.Require().NoError(err)
	s.Equal("origin", repo)

	repo, err = getRemoteFromOutput(out, "evergreen-ci", "evergreen")
	s.Require().NoError(err)
	s.Equal("upstream", repo)

	// Case-insensitive search
	repo, err = getRemoteFromOutput(out, "zackarysantana", "evergreen")
	s.Require().NoError(err)
	s.Equal("origin", repo)

	// Case-insensitive search
	repo, err = getRemoteFromOutput(out, "Evergreen-CI", "evergreen")
	s.Require().NoError(err)
	s.Equal("upstream", repo)
}

func TestCountNumTasksToFinalize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.Clear(model.ProjectRefCollection))
	}()

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	pRef := &model.ProjectRef{
		Id:                    "testproject",
		VersionControlEnabled: utility.TruePtr(),
	}
	require.NoError(t, pRef.Insert())

	for tName, tCase := range map[string]func(t *testing.T, p *model.Project){
		"SucceedsWithAllTasks": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Variants: []string{"all"},
				Tasks:    []string{"all"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 3)
		},
		"SucceedsWithSingleSelectedVariant": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Variants: []string{"v2"},
				Tasks:    []string{"all"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 2)
		},
		"SucceedsWithDisplayTask": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
  - name: "t2"
  - name: "t3"
  display_tasks:
  - name: "displayTask"
    execution_tasks:
    - "t2"
    - "t3"
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Variants: []string{"all"},
				Tasks:    []string{"all"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 5)
		},
		"SkipsDisabledTask": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
- name: "v2"
  tasks:
  - name: "t1"
    disable: true
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Variants: []string{"all"},
				Tasks:    []string{"all"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 2)
		},
		"SkipsTaskNotInAllowedRequesters": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    allowed_requesters: ["github_tag"]
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Variants: []string{"all"},
				Tasks:    []string{"all"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 2)
		},
		"SucceedsWithAlias": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"

patch_aliases:
  - alias: alias1
    variant: v2
    task: ".*"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				Alias: "alias1",
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 2)
		},
		"SucceedsWithRegexes": func(t *testing.T, p *model.Project) {
			yml := `
tasks:
- name: "t1"
- name: "t2"
- name: "t3"
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
- name: "v2"
  tasks:
  - name: "t1"
  - name: "t2"
`
			content := []byte(yml)
			_, err := model.LoadProjectInto(ctx, content, nil, "", p)
			require.NoError(t, err)
			params := &patchParams{
				RegexVariants: []string{".*"},
				RegexTasks:    []string{"t1"},
			}
			numTasks, err := countNumTasksToFinalize(p, params, content)
			require.NoError(t, err)
			assert.Equal(t, numTasks, 2)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			p := &model.Project{Identifier: pRef.Id}
			tCase(t, p)
		})
	}
}
