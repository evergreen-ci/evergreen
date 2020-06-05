package generator

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGolangGenerate(t *testing.T) {
	checkTask := func(t *testing.T, g *Golang, task *shrub.Task) {
		require.Len(t, task.Commands, 2)

		getProjectCmd := task.Commands[0]
		assert.Equal(t, shrub.CmdGetProject{}.Name(), getProjectCmd.CommandName)
		projectPath, err := g.RelProjectPath()
		require.NoError(t, err)
		assert.Equal(t, projectPath, getProjectCmd.Params["directory"])

		scriptingCmd := task.Commands[1]
		assert.Equal(t, shrub.CmdSubprocessScripting{}.Name(), scriptingCmd.CommandName)
		gopath, err := g.RelGopath()
		require.NoError(t, err)
		assert.Equal(t, gopath, scriptingCmd.Params["harness_path"])
		assert.Equal(t, g.WorkingDirectory, scriptingCmd.Params["working_dir"])
		assert.Equal(t, projectPath, scriptingCmd.Params["test_dir"])
		env, ok := scriptingCmd.Params["env"].(map[string]interface{})
		require.True(t, ok)
		assert.EqualValues(t, g.Environment["GOROOT"], env["GOROOT"])
	}

	checkTaskInTaskGroup := func(t *testing.T, g *Golang, task *shrub.Task) {
		require.Len(t, task.Commands, 1)
		scriptingCmd := task.Commands[0]
		assert.Equal(t, shrub.CmdSubprocessScripting{}.Name(), scriptingCmd.CommandName)
		gopath, err := g.RelGopath()
		require.NoError(t, err)
		assert.Equal(t, gopath, scriptingCmd.Params["harness_path"])
		assert.Equal(t, g.WorkingDirectory, scriptingCmd.Params["working_dir"])
		projectPath, err := g.RelProjectPath()
		require.NoError(t, err)
		assert.Equal(t, projectPath, scriptingCmd.Params["test_dir"])
		env, ok := scriptingCmd.Params["env"].(map[string]interface{})
		require.True(t, ok)
		assert.EqualValues(t, g.Environment["GOROOT"], env["GOROOT"])
	}

	checkVariantForTasks := func(t *testing.T, variant *shrub.Variant, distros []string, taskNames []string) {
		assert.Equal(t, distros, variant.DistroRunOn)
		tasksFound := make([]bool, len(taskNames))
		assert.Len(t, variant.TaskSpecs, len(taskNames))
		for _, task := range variant.TaskSpecs {
			for i, taskName := range taskNames {
				if task.Name == taskName {
					tasksFound[i] = true
				}
			}
		}
		for i, found := range tasksFound {
			assert.True(t, found, "missing task %d", i)
		}
	}
	for testName, testCase := range map[string]func(t *testing.T, g *Golang){
		"Succeeds": func(t *testing.T, g *Golang) {
			conf, err := g.Generate()
			require.NoError(t, err)

			expectedTasks := [][]string{
				{"variant1", "path1"},
				{"variant1", "name2"},
				{"variant2", "name2"},
			}
			require.Len(t, conf.Tasks, len(expectedTasks))

			for _, parts := range expectedTasks {
				task := conf.Task(getTaskName(parts...))
				checkTask(t, g, task)
			}

			variant := conf.Variant("variant1")
			checkVariantForTasks(t, variant, g.Variants[0].Distros, []string{
				getTaskName(expectedTasks[0]...), getTaskName(expectedTasks[1]...),
			})

			variant = conf.Variant("variant2")
			checkVariantForTasks(t, variant, g.Variants[1].Distros, []string{
				getTaskName(expectedTasks[2]...),
			})
		},
		"CreatesTaskGroup": func(t *testing.T, g *Golang) {
			numTasks := minTasksForTaskGroup
			g.Packages = nil
			for i := 1; i <= numTasks; i++ {
				g.Packages = append(g.Packages, model.GolangPackage{
					Name: "name" + string(i),
					Path: "path" + string(i),
					Tags: []string{"tag"},
				})
			}
			g.Variants = []model.GolangVariant{
				{
					VariantDistro: model.VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: model.GolangVariantParameters{
						Packages: []model.GolangVariantPackage{
							{Tag: "tag"},
						},
					},
				},
			}

			conf, err := g.Generate()
			require.NoError(t, err)

			var taskNames []string
			for _, gp := range g.Packages {
				taskName := getTaskName(g.Variants[0].Name, gp.Tags[0], gp.Name)
				taskNames = append(taskNames, taskName)
				task := conf.Task(taskName)
				checkTaskInTaskGroup(t, g, task)
			}

			taskGroup := conf.TaskGroup(getTaskGroupName(g.Variants[0].Name))

			assert.Equal(t, numTasks/2, taskGroup.MaxHosts)
			assert.Len(t, taskGroup.Tasks, numTasks)
			require.Len(t, taskGroup.SetupTask, 1)
			getProjectCmd := taskGroup.SetupTask[0]
			assert.Equal(t, shrub.CmdGetProject{}.Name(), getProjectCmd.CommandName)
			projectPath, err := g.RelProjectPath()
			require.NoError(t, err)
			assert.Equal(t, projectPath, getProjectCmd.Params["directory"])
			assert.Subset(t, taskGroup.Tasks, taskNames)

			variant := conf.Variant(g.Variants[0].Name)
			checkVariantForTasks(t, variant, g.Variants[0].Distros, []string{taskGroup.GroupName})
		},
		"CreatesTaskFromTags": func(t *testing.T, g *Golang) {
			g.Packages = []model.GolangPackage{
				{
					Path: "path1",
					Tags: []string{"tag"},
				},
				{
					Name: "name2",
					Path: "path2",
					Tags: []string{"tag"},
				},
			}
			g.Variants = []model.GolangVariant{
				{
					VariantDistro: model.VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					GolangVariantParameters: model.GolangVariantParameters{
						Packages: []model.GolangVariantPackage{
							{Tag: "tag"},
						},
					},
				},
			}

			conf, err := g.Generate()
			require.NoError(t, err)

			require.Len(t, conf.Tasks, 2)
			task1 := conf.Task(getTaskName(g.Variants[0].Name, g.Packages[0].Tags[0], g.Packages[0].Path))
			checkTask(t, g, task1)

			task2 := conf.Task(getTaskName(g.Variants[0].Name, g.Packages[0].Tags[0], g.Packages[1].Name))
			checkTask(t, g, task2)

			variant := conf.Variant(g.Variants[0].Name)
			expectedTasks := []string{task1.Name, task2.Name}
			checkVariantForTasks(t, variant, g.Variants[0].Distros, expectedTasks)
		},
		"FailsWithVariantReferenceToNonexistentPackage": func(t *testing.T, g *Golang) {
			g.Variants = append(g.Variants, model.GolangVariant{
				VariantDistro: model.VariantDistro{
					Name: "newVariant",
				},
				GolangVariantParameters: model.GolangVariantParameters{
					Packages: []model.GolangVariantPackage{
						{Name: "nonexistent"},
					},
				},
			})
			conf, err := g.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
		"FailsWithVariantReferenceToNonexistentPath": func(t *testing.T, g *Golang) {
			g.Variants = append(g.Variants, model.GolangVariant{
				VariantDistro: model.VariantDistro{
					Name: "newVariant",
				},
				GolangVariantParameters: model.GolangVariantParameters{
					Packages: []model.GolangVariantPackage{
						{Path: "nonexistent"},
					},
				},
			})
			conf, err := g.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
		"FailsWithVariantReferenceToNonexistentTag": func(t *testing.T, g *Golang) {
			g.Variants = append(g.Variants, model.GolangVariant{
				VariantDistro: model.VariantDistro{
					Name: "newVariant",
				},
				GolangVariantParameters: model.GolangVariantParameters{
					Packages: []model.GolangVariantPackage{
						{Tag: "nonexistent"},
					},
				},
			})
			conf, err := g.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
		"FailsWithGOPATHNotWithinWorkingDirectory": func(t *testing.T, g *Golang) {
			absGopath, err := filepath.Abs(filepath.Join("/path", "outside", "working", "directory"))
			require.NoError(t, err)
			g.Environment["GOPATH"] = util.ConsistentFilepath(absGopath)
			conf, err := g.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			rootPackage := util.ConsistentFilepath("github.com", "fake_user", "fake_repo")
			gopath := "gopath"

			mg := model.Golang{
				Environment: map[string]string{
					"GOPATH": gopath,
					"GOROOT": "some_goroot",
				},
				RootPackage: rootPackage,
				Packages: []model.GolangPackage{
					{
						Path: "path1",
					},
					{
						Name: "name2",
						Path: "path2",
					},
				},
				Variants: []model.GolangVariant{
					{
						VariantDistro: model.VariantDistro{
							Name:    "variant1",
							Distros: []string{"distro1"},
						},
						GolangVariantParameters: model.GolangVariantParameters{
							Packages: []model.GolangVariantPackage{
								{Path: "path1"},
								{Name: "name2"},
							},
						},
					},
					{
						VariantDistro: model.VariantDistro{
							Name:    "variant2",
							Distros: []string{"distro2"},
						},
						GolangVariantParameters: model.GolangVariantParameters{
							Packages: []model.GolangVariantPackage{
								{Name: "name2"},
							},
						},
					},
				},
				WorkingDirectory: util.ConsistentFilepath(filepath.Dir(gopath)),
			}

			g := NewGolang(mg)
			require.NoError(t, g.Validate())

			testCase(t, g)
		})
	}
}
