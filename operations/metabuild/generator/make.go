package generator

import (
	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/pkg/errors"
)

// Make represents an evergreen config generator for Make-based projects.
type Make struct {
	model.Make
}

// NewMake returns a generator for Make.
func NewMake(m model.Make) *Make {
	return &Make{
		Make: m,
	}
}

func (m *Make) Generate() (*shrub.Configuration, error) {
	conf, err := shrub.BuildConfiguration(func(c *shrub.Configuration) {
		for _, mv := range m.Variants {
			variant := c.Variant(mv.Name)
			variant.DistroRunOn = mv.Distros

			var tasksForVariant []*shrub.Task
			for _, mvt := range mv.Tasks {
				tasks, err := m.GetTasksFromRef(mvt)
				if err != nil {
					panic(err)
				}
				newTasks, err := m.generateVariantTasksForRef(c, mv, tasks)
				if err != nil {
					panic(err)
				}
				tasksForVariant = append(tasksForVariant, newTasks...)
			}

			getProjectCmd := shrub.CmdGetProject{
				Directory: m.WorkingDirectory,
			}

			if len(tasksForVariant) >= minTasksForTaskGroup {
				tg := c.TaskGroup(getTaskGroupName(mv.Name)).SetMaxHosts(len(tasksForVariant) / 2)
				tg.SetupTask = shrub.CommandSequence{getProjectCmd.Resolve()}

				for _, task := range tasksForVariant {
					_ = tg.Task(task.Name)
				}
				_ = variant.AddTasks(tg.GroupName)
			} else {
				for _, task := range tasksForVariant {
					task.Commands = append([]*shrub.CommandDefinition{getProjectCmd.Resolve()}, task.Commands...)
					_ = variant.AddTasks(task.Name)
				}
			}
		}
	})

	if err != nil {
		return nil, errors.Wrap(err, "generating evergreen configuration")
	}
	return conf, nil
}

// generateVariantTasksForRef generates the tasks for the given variant from a
// single task reference in the given variant.
func (m *Make) generateVariantTasksForRef(c *shrub.Configuration, mv model.MakeVariant, mts []model.MakeTask) ([]*shrub.Task, error) {
	var tasks []*shrub.Task
	for _, mt := range mts {
		cmds, err := m.taskCmds(mv, mt)
		if err != nil {
			return nil, errors.Wrap(err, "generating commands to run")
		}
		tasks = append(tasks, c.Task(getTaskName(mv.Name, mt.Name)).Command(cmds...))
	}
	return tasks, nil
}

// taskCmds returns the commands that should be executed for the given task
// within the given variant.
func (m *Make) taskCmds(mv model.MakeVariant, mt model.MakeTask) ([]shrub.Command, error) {
	env := model.MergeEnvironments(m.Environment, mv.Environment, mt.Environment)
	var cmds []shrub.Command
	for _, target := range mt.Targets {
		targetNames, err := m.GetTargetsFromRef(target)
		if err != nil {
			return nil, errors.Wrap(err, "resolving commands for targets")
		}
		opts := target.Options.Merge(mt.Options, mv.Options)
		for _, targetName := range targetNames {
			cmds = append(cmds, &shrub.CmdExec{
				Binary:           "make",
				Args:             append(opts, targetName),
				Env:              env,
				WorkingDirectory: m.WorkingDirectory,
			})
		}
		reportCmds, err := fileReportCmds(target.Reports...)
		if err != nil {
			return nil, errors.Wrapf(err, "resolving target file report commands")
		}
		cmds = append(cmds, reportCmds...)
	}

	reportCmds, err := fileReportCmds(mt.Reports...)
	if err != nil {
		return nil, errors.Wrap(err, "resolving task file report commands")
	}
	cmds = append(cmds, reportCmds...)

	return cmds, nil
}
