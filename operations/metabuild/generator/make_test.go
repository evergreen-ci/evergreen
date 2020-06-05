package generator

import (
	"testing"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeGenerate(t *testing.T) {
	checkTask := func(t *testing.T, m *Make, task *shrub.Task, targets, opts []string) {
		require.Len(t, task.Commands, len(targets)+1)

		getProjectCmd := task.Commands[0]
		assert.Equal(t, shrub.CmdGetProject{}.Name(), getProjectCmd.CommandName)
		assert.Equal(t, m.WorkingDirectory, getProjectCmd.Params["directory"])

		for i, execCmd := range task.Commands[1:] {
			assert.Equal(t, shrub.CmdExec{}.Name(), execCmd.CommandName)
			assert.Equal(t, m.WorkingDirectory, execCmd.Params["working_dir"])
			execArgs, ok := execCmd.Params["args"].([]interface{})
			require.True(t, ok)
			assert.Subset(t, execArgs, opts)
			assert.Contains(t, execArgs, targets[i])
		}
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

	checkTaskInTaskGroup := func(t *testing.T, m *Make, task *shrub.Task, targets []string, opts []string) {
		require.Len(t, task.Commands, len(targets))
		for i, execCmd := range task.Commands {
			assert.Equal(t, shrub.CmdExec{}.Name(), execCmd.CommandName)
			assert.Equal(t, m.WorkingDirectory, execCmd.Params["working_dir"])
			execArgs, ok := execCmd.Params["args"].([]interface{})
			require.True(t, ok)
			assert.Subset(t, execArgs, opts)
			assert.Contains(t, execArgs, targets[i])
		}
	}

	for testName, testCase := range map[string]func(t *testing.T, m *Make){
		"Succeeds": func(t *testing.T, m *Make) {
			conf, err := m.Generate()
			require.NoError(t, err)

			assert.Len(t, conf.Tasks, 2)

			task := conf.Task(getTaskName(m.Variants[0].Name, m.Tasks[0].Name))
			targets := []string{m.Tasks[0].Targets[0].Name}
			checkTask(t, m, task, targets, nil)

			task = conf.Task(getTaskName(m.Variants[1].Name, m.Tasks[1].Name))
			targets = append([]string{m.Tasks[1].Targets[0].Name}, m.TargetSequences[0].Targets...)
			checkTask(t, m, task, targets, nil)

			variant := conf.Variant(m.Variants[0].Name)
			expectedTasks := []string{getTaskName(m.Variants[0].Name, m.Tasks[0].Name)}
			checkVariantForTasks(t, variant, m.Variants[0].Distros, expectedTasks)

			variant = conf.Variant(m.Variants[1].Name)
			expectedTasks = []string{getTaskName(m.Variants[1].Name, m.Tasks[1].Name)}
			checkVariantForTasks(t, variant, m.Variants[1].Distros, expectedTasks)
		},
		"CreatesTaskGroup": func(t *testing.T, m *Make) {
			numTasks := minTasksForTaskGroup
			m.Tasks = nil
			for i := 1; i <= numTasks; i++ {
				m.Tasks = append(m.Tasks, model.MakeTask{
					Name: "task" + string(i),
					Targets: []model.MakeTaskTarget{
						{Name: "target" + string(i)},
					},
					Tags: []string{"tag"},
				})
			}
			m.Variants = []model.MakeVariant{
				{
					VariantDistro: model.VariantDistro{
						Name:    "variant",
						Distros: []string{"distro"},
					},
					MakeVariantParameters: model.MakeVariantParameters{
						Tasks: []model.MakeVariantTask{
							{Tag: "tag"},
						},
					},
				},
			}

			conf, err := m.Generate()
			require.NoError(t, err)

			var taskNames []string
			for _, mt := range m.Tasks {
				taskName := getTaskName(m.Variants[0].Name, mt.Name)
				taskNames = append(taskNames, taskName)
				task := conf.Task(taskName)
				checkTaskInTaskGroup(t, m, task, []string{mt.Targets[0].Name}, nil)
			}

			taskGroup := conf.TaskGroup(getTaskGroupName(m.Variants[0].Name))
			require.Len(t, taskGroup.SetupTask, 1)
			getProjectCmd := taskGroup.SetupTask[0]
			assert.Equal(t, shrub.CmdGetProject{}.Name(), getProjectCmd.CommandName)
			assert.Equal(t, m.WorkingDirectory, getProjectCmd.Params["directory"])
			assert.Equal(t, numTasks/2, taskGroup.MaxHosts)
			assert.Len(t, taskGroup.Tasks, numTasks)
			assert.Subset(t, taskGroup.Tasks, taskNames)

			variant := conf.Variant(m.Variants[0].Name)
			checkVariantForTasks(t, variant, m.Variants[0].Distros, []string{taskGroup.GroupName})
		},
		"CreatesTasksFromTags": func(t *testing.T, m *Make) {
			m.Tasks = []model.MakeTask{
				{
					Name: "task1",
					Targets: []model.MakeTaskTarget{
						{Name: "target1"},
					},
					Tags: []string{"tag"},
				},
				{
					Name: "task2",
					Targets: []model.MakeTaskTarget{
						{Name: "target2"},
					},
					Tags: []string{"tag"},
				},
			}
			m.Variants = []model.MakeVariant{
				{
					VariantDistro: model.VariantDistro{
						Name:    "variant1",
						Distros: []string{"distro1"},
					},
					MakeVariantParameters: model.MakeVariantParameters{
						Tasks: []model.MakeVariantTask{
							{Tag: "tag"},
						},
					},
				},
			}

			conf, err := m.Generate()
			require.NoError(t, err)

			task1 := conf.Task(getTaskName(m.Variants[0].Name, m.Tasks[0].Name))
			targets1 := []string{m.Tasks[0].Targets[0].Name}
			checkTask(t, m, task1, targets1, nil)

			task2 := conf.Task(getTaskName(m.Variants[0].Name, m.Tasks[1].Name))
			targets2 := []string{m.Tasks[1].Targets[0].Name}
			checkTask(t, m, task2, targets2, nil)

			expectedTasks := []string{task1.Name, task2.Name}
			variant := conf.Variant(m.Variants[0].Name)
			checkVariantForTasks(t, variant, m.Variants[0].Distros, expectedTasks)
		},
		"FailsWithTaskReferenceToNonexistentTargetSequence": func(t *testing.T, m *Make) {
			m.Tasks = append(m.Tasks, model.MakeTask{
				Name: "newTask",
				Targets: []model.MakeTaskTarget{
					{Sequence: "nonexistent"},
				},
			})
			m.Variants = append(m.Variants, model.MakeVariant{
				VariantDistro: model.VariantDistro{
					Name:    "newVariant",
					Distros: []string{"newDistro"},
				},
				MakeVariantParameters: model.MakeVariantParameters{
					Tasks: []model.MakeVariantTask{
						{Name: "newTask"},
					},
				},
			})
			conf, err := m.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
		"FailsWithVariantReferenceToNonexistentTask": func(t *testing.T, m *Make) {
			m.Variants = append(m.Variants, model.MakeVariant{
				VariantDistro: model.VariantDistro{
					Name:    "newVariant",
					Distros: []string{"newDistro"},
				},
				MakeVariantParameters: model.MakeVariantParameters{
					Tasks: []model.MakeVariantTask{
						{Name: "nonexistent"},
					},
				},
			})
			conf, err := m.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
		"FailsWithVariantReferenceToNonexistentTag": func(t *testing.T, m *Make) {
			m.Variants = append(m.Variants, model.MakeVariant{
				VariantDistro: model.VariantDistro{
					Name:    "newVariant",
					Distros: []string{"newDistro"},
				},
				MakeVariantParameters: model.MakeVariantParameters{
					Tasks: []model.MakeVariantTask{
						{Tag: "nonexistent"},
					},
				},
			})
			conf, err := m.Generate()
			assert.Error(t, err)
			assert.Zero(t, conf)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mm := model.Make{
				TargetSequences: []model.MakeTargetSequence{
					{
						Name:    "sequence1",
						Targets: []string{"target2", "target3"},
					},
				},
				Tasks: []model.MakeTask{
					{
						Name: "task1",
						Targets: []model.MakeTaskTarget{
							{Name: "target1"},
						},
					},
					{
						Name: "task2",
						Targets: []model.MakeTaskTarget{
							{Name: "target1"},
							{Sequence: "sequence1"},
						},
						Tags: []string{"tag1"},
					},
				},
				Variants: []model.MakeVariant{
					{
						VariantDistro: model.VariantDistro{
							Name:    "variant1",
							Distros: []string{"distro1"},
						},
						MakeVariantParameters: model.MakeVariantParameters{
							Tasks: []model.MakeVariantTask{
								{Name: "task1"},
							},
						},
					},
					{
						VariantDistro: model.VariantDistro{
							Name:    "variant2",
							Distros: []string{"distro2"},
						},
						MakeVariantParameters: model.MakeVariantParameters{
							Tasks: []model.MakeVariantTask{
								{Name: "task2"},
							},
						},
					},
				},
				WorkingDirectory: "working_dir",
			}

			m := NewMake(mm)
			require.NoError(t, m.Validate())

			testCase(t, m)
		})
	}
}
