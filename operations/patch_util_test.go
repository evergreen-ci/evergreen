package operations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
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
   - mytask2
`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci"}
	s.Require().NoError(pp.loadAlias(s.T().Context(), conf))
	s.Require().NoError(pp.loadVariants(s.T().Context(), conf))
	s.Require().NoError(pp.loadTasks(s.T().Context(), conf))

	s.Equal("testing", pp.Alias)
	s.Nil(pp.Variants)
	s.Nil(pp.Tasks)

	// If tasks/variants are specified, then we should not set the alias or task/variant defaults.
	pp = patchParams{Project: "mci", RegexTasks: []string{".*"}, RegexVariants: []string{".*"}}
	s.Require().NoError(pp.loadAlias(s.T().Context(), conf))
	s.Require().NoError(pp.loadVariants(s.T().Context(), conf))
	s.Require().NoError(pp.loadTasks(s.T().Context(), conf))

	s.Empty(pp.Alias)
	s.Empty(pp.Tasks)
	s.Empty(pp.Variants)
}

func (s *PatchUtilTestSuite) TestSelectNoneSkipsDefaults() {
	fileContents := `projects:
- name: mci
  default: true
  alias: testing
  variants:
   - myvariant1
  tasks:
   - mytask1
`

	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	pp := patchParams{Project: "mci", SelectNone: true}
	pp.setNonRepeatedDefaults(s.T().Context(), conf)

	s.Empty(pp.Alias)
	s.Empty(pp.Tasks)
	s.Empty(pp.Variants)
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

	s.Require().NoError(pp.loadAlias(s.T().Context(), conf))
	s.Require().NoError(pp.loadVariants(s.T().Context(), conf))
	s.Require().NoError(pp.loadTasks(s.T().Context(), conf))

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

	s.Require().NoError(pp.loadAlias(s.T().Context(), conf))
	s.Require().NoError(pp.loadVariants(s.T().Context(), conf))
	s.Require().NoError(pp.loadTasks(s.T().Context(), conf))

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

	s.Require().NoError(pp.loadAlias(s.T().Context(), conf))
	s.Require().NoError(pp.loadVariants(s.T().Context(), conf))
	s.Require().NoError(pp.loadTasks(s.T().Context(), conf))

	s.Zero(pp.Alias)
	s.Contains(pp.Variants, "myvariant1")
	s.Contains(pp.Variants, "myvariant2")
	s.Contains(pp.Tasks, "mytask1")
	s.Contains(pp.Tasks, "mytask2")
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsLoadsExplicitAlias() {
	pp := patchParams{
		Project: "project",
		Aliases: []string{"duck"},
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

	pp.setNonRepeatedDefaults(s.T().Context(), conf)

	s.Equal("duck", pp.Alias, "alias should not be defaulted")
	s.Empty(pp.Variants, "variants should not be defaulted for explicit alias")
	s.Empty(pp.Tasks, "tasks should not be defaulted for explicit alias")
}

func (s *PatchUtilTestSuite) TestNonRepeatedDefaultsWithLocalAliasOverridesOtherDefaults() {
	pp := patchParams{
		Project: "project",
		Aliases: []string{"duck"},
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

	pp.setNonRepeatedDefaults(s.T().Context(), conf)

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

	pp.setNonRepeatedDefaults(s.T().Context(), conf)

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

	pp.setNonRepeatedDefaults(s.T().Context(), conf)

	s.Empty(pp.Alias, "should not set an alias when there is no default")
	s.ElementsMatch([]string{"default-bv0", "default-bv1"}, pp.Variants, "variants should be defaulted")
	s.ElementsMatch([]string{"default-task0", "default-task1"}, pp.Tasks, "tasks should be defaulted")
}

func (s *PatchUtilTestSuite) TestMultipleLocalAliasesExpandIntoTasksAndVariants() {
	pp := patchParams{
		Project:     "project",
		Aliases:     []string{"duck", "goose"},
		SkipConfirm: true,
	}
	conf := &ClientSettings{
		Projects: []ClientProjectConf{
			{
				Name: "project",
				LocalAliases: []model.ProjectAlias{
					{Alias: "duck", Variant: "bv0", Task: "task0"},
					{Alias: "goose", Variant: "bv1", Task: "task1"},
				},
			},
		},
	}

	pp.setNonRepeatedDefaults(s.T().Context(), conf)

	s.Empty(pp.Aliases, "local aliases should be consumed during expansion")
	s.Empty(pp.Alias, "no server-side alias should remain")
	s.True(pp.isUsingLocalAlias)
	s.ElementsMatch([]string{"bv0", "bv1"}, pp.RegexVariants)
	s.ElementsMatch([]string{"task0", "task1"}, pp.RegexTasks)
}

func (s *PatchUtilTestSuite) TestMultipleServerAliasesSubmitted() {
	// Multiple server-side aliases are submitted as-is (no longer rejected).
	pp := patchParams{Aliases: []string{"server-alias-1", "server-alias-2"}}
	s.Equal([]string{"server-alias-1", "server-alias-2"}, pp.submissionAliases())

	// A single resolved alias still submits as a one-element list.
	pp = patchParams{Alias: "solo"}
	s.Equal([]string{"solo"}, pp.submissionAliases())
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

func (s *PatchUtilTestSuite) TestValidatePatchCommand() {
	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	// uncommitted and ref should not be combined
	p := patchParams{
		Project:     "mci",
		Finalize:    true,
		Uncommitted: true,
		Ref:         "myref",
	}

	assertRef, err := p.validatePatchCommand(context.Background(), conf, nil, nil)
	s.Require().Error(err, "expected error due to uncommitted and ref being set")
	s.Nil(assertRef)

	// conf.Uncommitted should be considered
	p = patchParams{
		Project:  "mci",
		Finalize: true,
		Ref:      "myref",
	}

	conf.UncommittedChanges = true
	assertRef, err = p.validatePatchCommand(context.Background(), conf, nil, nil)
	s.Require().Error(err, "expected error due to conf.uncommittedChanges and ref being set")
	s.Nil(assertRef)

}

func (s *PatchUtilTestSuite) TestLoadProject() {
	// Test that loadProject uses default when no project is specified
	fileContents := `projects:
- name: evergreen
  default: true
- name: mci
`
	err := os.WriteFile(s.testConfigFile, []byte(fileContents), 0644)
	s.Require().NoError(err)

	conf, err := NewClientSettings(s.testConfigFile)
	s.Require().NoError(err)

	// Test that loadProject sets the default project when none is specified
	defaultProject := patchParams{}
	err = defaultProject.loadProject(s.T().Context(), conf)
	s.NoError(err, "loadProject should not error")
	s.Equal("evergreen", defaultProject.Project, "loadProject should set project to default")

	// Test that loadProject errors when no project is specified and no default exists
	for i := range conf.Projects {
		conf.Projects[i].Default = false
	}

	emptyProject := patchParams{}
	err = emptyProject.loadProject(s.T().Context(), conf)
	s.Error(err, "loadProject should error when no default exists")
	s.Contains(err.Error(), "Need to specify a project", "error message should indicate project is required")
	s.Empty(emptyProject.Project, "loadProject should leave project empty when no default exists")

	// Test that loadProject succeeds when valid project is specified
	validProject := patchParams{
		Project: "mci",
	}
	err = validProject.loadProject(s.T().Context(), conf)
	s.NoError(err, "loadProject should not error when valid project is specified")
	s.Equal("mci", validProject.Project, "loadProject should preserve the specified project")
}

func TestGetLocalModuleIncludes(t *testing.T) {
	tempDir := t.TempDir()
	moduleDir := filepath.Join(tempDir, "mymodule")
	err := os.MkdirAll(filepath.Join(moduleDir, "evergreen", "my_project", "master"), 0755)
	require.NoError(t, err)

	testFiles := []struct {
		path    string
		content string
	}{
		{"evergreen/my_project/master/base.yml", "blah"},
		{"evergreen/my_project/shared_tasks.yml", "blah"},
		{"evergreen/my_project/master/compiles.yml", "blah"},
		{"evergreen/my_project/master/variants.yml", "blah"},
		{"evergreen/my_project/master/genny_tasks.yml", "blah"},
	}

	for _, file := range testFiles {
		filePath := filepath.Join(moduleDir, file.path)
		err := os.WriteFile(filePath, []byte(file.content), 0644)
		require.NoError(t, err)
	}

	t.Run("AllIncludesProcessedWhenModulePathProvided", func(t *testing.T) {
		projectYAML := `
include:
  - filename: evergreen/my_project/master/base.yml
    module: mymodule
  - filename: evergreen/my_project/shared_tasks.yml
    module: mymodule
  - filename: evergreen/my_project/master/compiles.yml
    module: mymodule
  - filename: evergreen/my_project/master/variants.yml
    module: mymodule
  - filename: evergreen/my_project/master/genny_tasks.yml
    module: mymodule
`

		projectFile := filepath.Join(tempDir, "project.yml")
		err := os.WriteFile(projectFile, []byte(projectYAML), 0644)
		require.NoError(t, err)

		params := &patchParams{
			Project:     "test-project",
			SkipConfirm: true,
		}

		conf := &ClientSettings{}
		modulePathCache := map[string]string{
			"mymodule": moduleDir,
		}

		includes, err := getLocalModuleIncludes(t.Context(), params, conf, projectFile, "", modulePathCache)
		require.NoError(t, err)

		assert.Len(t, includes, 5)
		for i, include := range includes {
			assert.Equal(t, "mymodule", include.Module)
			assert.Equal(t, testFiles[i].path, include.FileName)
			assert.Equal(t, testFiles[i].content, string(include.FileContent))
		}
	})

	t.Run("NoIncludesProcessedWhenModulePathNotProvided", func(t *testing.T) {
		projectYAML := `
include:
  - filename: evergreen/my_project/master/base.yml
    module: mymodule
  - filename: evergreen/my_project/shared_tasks.yml
    module: mymodule
  - filename: evergreen/my_project/master/compiles.yml
    module: mymodule
`

		projectFile := filepath.Join(tempDir, "project-no-path.yml")
		err := os.WriteFile(projectFile, []byte(projectYAML), 0644)
		require.NoError(t, err)

		params := &patchParams{
			Project:     "test-project",
			SkipConfirm: true,
		}

		conf := &ClientSettings{}
		modulePathCache := map[string]string{}

		includes, err := getLocalModuleIncludes(t.Context(), params, conf, projectFile, "", modulePathCache)

		require.NoError(t, err)
		assert.Len(t, includes, 0)
	})

	t.Run("MultipleModules", func(t *testing.T) {
		multiModuleYAML := `
include:
  - filename: file1.yml
    module: module1
  - filename: file2.yml
    module: module1
  - filename: file3.yml
    module: module2
  - filename: file4.yml
    module: module2
  - filename: file5.yml
    module: module1
`

		module1Dir := filepath.Join(tempDir, "module1")
		module2Dir := filepath.Join(tempDir, "module2")
		err := os.MkdirAll(module1Dir, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(module2Dir, 0755)
		require.NoError(t, err)

		for i := 1; i <= 5; i++ {
			var dir string
			if i <= 2 || i == 5 {
				dir = module1Dir
			} else {
				dir = module2Dir
			}
			filePath := filepath.Join(dir, fmt.Sprintf("file%d.yml", i))
			content := fmt.Sprintf("content: file%d", i)
			err := os.WriteFile(filePath, []byte(content), 0644)
			require.NoError(t, err)
		}

		multiModuleFile := filepath.Join(tempDir, "multi-module.yml")
		err = os.WriteFile(multiModuleFile, []byte(multiModuleYAML), 0644)
		require.NoError(t, err)

		params := &patchParams{
			Project:     "test-project",
			SkipConfirm: true,
		}

		conf := &ClientSettings{}
		modulePathCache := map[string]string{
			"module1": module1Dir,
			"module2": module2Dir,
		}

		includes, err := getLocalModuleIncludes(t.Context(), params, conf, multiModuleFile, "", modulePathCache)
		require.NoError(t, err)

		assert.Len(t, includes, 5)

		module1Count := 0
		module2Count := 0
		for _, inc := range includes {
			if inc.Module == "module1" {
				module1Count++
			} else if inc.Module == "module2" {
				module2Count++
			}
		}
		assert.Equal(t, 3, module1Count)
		assert.Equal(t, 2, module2Count)
	})

	t.Run("PartialProcessingWhenSomeModulesHavePaths", func(t *testing.T) {
		multiModuleYAML := `
include:
  - filename: file1.yml
    module: module1
  - filename: file2.yml
    module: module1
  - filename: file3.yml
    module: module2
  - filename: file4.yml
    module: module2
`

		module1Dir := filepath.Join(tempDir, "partial-module1")
		module2Dir := filepath.Join(tempDir, "partial-module2")
		err := os.MkdirAll(module1Dir, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(module2Dir, 0755)
		require.NoError(t, err)

		for i := 1; i <= 4; i++ {
			var dir string
			if i <= 2 {
				dir = module1Dir
			} else {
				dir = module2Dir
			}
			filePath := filepath.Join(dir, fmt.Sprintf("file%d.yml", i))
			content := fmt.Sprintf("partial: file%d", i)
			err := os.WriteFile(filePath, []byte(content), 0644)
			require.NoError(t, err)
		}

		partialModuleFile := filepath.Join(tempDir, "partial-module.yml")
		err = os.WriteFile(partialModuleFile, []byte(multiModuleYAML), 0644)
		require.NoError(t, err)

		params := &patchParams{
			Project:     "test-project",
			SkipConfirm: true,
		}

		conf := &ClientSettings{}
		modulePathCache := map[string]string{
			"module1": module1Dir,
		}

		includes, err := getLocalModuleIncludes(t.Context(), params, conf, partialModuleFile, "", modulePathCache)

		require.NoError(t, err)

		assert.Len(t, includes, 2)

		for _, inc := range includes {
			assert.Equal(t, "module1", inc.Module)
			assert.Contains(t, []string{"file1.yml", "file2.yml"}, inc.FileName)
		}
	})

	t.Run("CLIOverridePathTakesPrecedence", func(t *testing.T) {
		originalDir := filepath.Join(tempDir, "original_module")
		overrideDir := filepath.Join(tempDir, "override_module")
		require.NoError(t, os.MkdirAll(originalDir, 0755))
		require.NoError(t, os.MkdirAll(overrideDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(originalDir, "config.yml"), []byte("original"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(overrideDir, "config.yml"), []byte("override"), 0644))

		projectYAML := `
include:
  - filename: config.yml
    module: mymodule
`
		projectFile := filepath.Join(tempDir, "project-override.yml")
		require.NoError(t, os.WriteFile(projectFile, []byte(projectYAML), 0644))

		params := &patchParams{
			Project:     "test-project",
			SkipConfirm: true,
		}
		conf := &ClientSettings{
			Projects: []ClientProjectConf{
				{
					Name:        "test-project",
					ModulePaths: map[string]string{"mymodule": originalDir},
				},
			},
		}
		modulePathCache := conf.getModulePathsForProject("test-project")
		modulePathCache["mymodule"] = overrideDir

		includes, err := getLocalModuleIncludes(t.Context(), params, conf, projectFile, "", modulePathCache)
		require.NoError(t, err)
		require.Len(t, includes, 1)
		assert.Equal(t, "override", string(includes[0].FileContent))
	})
}

func TestParseModuleOverrides(t *testing.T) {
	t.Run("ValidSingleModule", func(t *testing.T) {
		result, err := parseModuleOverrides([]string{"dsi=/path/to/dsi"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"dsi": "/path/to/dsi"}, result)
	})

	t.Run("ValidMultipleModules", func(t *testing.T) {
		result, err := parseModuleOverrides([]string{"dsi=/path/to/dsi", "other=/path/to/other"})
		require.NoError(t, err)
		assert.Equal(t, "/path/to/dsi", result["dsi"])
		assert.Equal(t, "/path/to/other", result["other"])
	})

	t.Run("PathContainingEquals", func(t *testing.T) {
		result, err := parseModuleOverrides([]string{"dsi=/path/with=equals/dsi"})
		require.NoError(t, err)
		assert.Equal(t, "/path/with=equals/dsi", result["dsi"])
	})

	t.Run("EmptyInputReturnsEmptyMap", func(t *testing.T) {
		result, err := parseModuleOverrides([]string{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("MissingSeparatorShouldError", func(t *testing.T) {
		_, err := parseModuleOverrides([]string{"dsi"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "MODULE_NAME=PATH")
	})

	t.Run("EmptyModuleNameShouldError", func(t *testing.T) {
		_, err := parseModuleOverrides([]string{"=/path/to/dsi"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "MODULE_NAME=PATH")
	})

	t.Run("EmptyPathShouldError", func(t *testing.T) {
		_, err := parseModuleOverrides([]string{"dsi="})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "MODULE_NAME=PATH")
	})

	t.Run("DuplicateModuleShouldError", func(t *testing.T) {
		_, err := parseModuleOverrides([]string{"dsi=/path/one", "dsi=/path/two"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
	})
}

func TestGetModulePathUsesOverrideWithoutWritingConfig(t *testing.T) {
	params := &patchParams{
		Project:     "test-project",
		SkipConfirm: false,
	}
	conf := &ClientSettings{}
	modulePathCache := map[string]string{
		"dsi": "/override/path/dsi",
	}

	path, err := params.getModulePath(t.Context(), conf, "dsi", modulePathCache)
	require.NoError(t, err)
	assert.Equal(t, "/override/path/dsi", path)
	assert.Empty(t, conf.Projects)
}
