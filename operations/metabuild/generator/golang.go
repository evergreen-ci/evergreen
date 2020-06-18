package generator

import (
	"strings"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/jasper/metabuild/model"
	"github.com/pkg/errors"
)

// Golang represents a configuration for generating an evergreen configuration
// from a project that uses golang.
type Golang struct {
	model.Golang
}

func NewGolang(g model.Golang) *Golang {
	return &Golang{
		Golang: g,
	}
}

// Generate creates the evergreen configuration from the given golang build
// configuration.
func (g *Golang) Generate() (*shrub.Configuration, error) {
	conf, err := shrub.BuildConfiguration(func(c *shrub.Configuration) {
		for _, gv := range g.Variants {
			variant := c.Variant(gv.Name)
			variant.DistroRunOn = gv.Distros

			var tasksForVariant []*shrub.Task
			// Make one task per package in this variant. We cannot make one
			// task per package, because we have to account for variant-level
			// flags possibly overriding package-level flags, which requires
			// making separate tasks with different commands.
			for _, gvp := range gv.Packages {
				var gps []model.GolangPackage
				var pkgRef string
				gps, pkgRef, err := g.GetPackagesAndRef(gvp)
				if err != nil {
					panic(errors.Wrapf(err, "package definition for variant '%s'", gv.Name))
				}

				newTasks := g.generateVariantTasksForRef(c, gv, gps, pkgRef)
				tasksForVariant = append(tasksForVariant, newTasks...)
			}

			env := model.MergeEnvironments(g.Environment, gv.Environment)
			gopath := env["GOPATH"]
			projectPath := g.RelProjectPath(gopath)
			getProjectCmd := shrub.CmdGetProject{
				Directory: projectPath,
			}

			// Only use a task group for this variant if it meets the threshold
			// number of tasks. Otherwise, just run regular tasks for this
			// variant.
			if len(tasksForVariant) >= minTasksForTaskGroup {
				tg := c.TaskGroup(getTaskGroupName(gv.Name)).SetMaxHosts(len(tasksForVariant) / 2)
				tg.SetupGroup = shrub.CommandSequence{getProjectCmd.Resolve()}

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

func (g *Golang) generateVariantTasksForRef(c *shrub.Configuration, gv model.GolangVariant, gps []model.GolangPackage, pkgRef string) []*shrub.Task {
	var tasks []*shrub.Task

	for _, gp := range gps {
		scriptCmd := g.subprocessScriptingCmd(gv, gp)
		var taskName string
		if len(gps) > 1 {
			id := gp.Path
			if gp.Name != "" {
				id = gp.Name
			}
			taskName = getTaskName(gv.Name, pkgRef, id)
		} else {
			taskName = getTaskName(gv.Name, pkgRef)
		}
		tasks = append(tasks, c.Task(taskName).Command(scriptCmd))
	}

	return tasks
}

func (g *Golang) subprocessScriptingCmd(gv model.GolangVariant, gp model.GolangPackage) *shrub.CmdSubprocessScripting {
	env := model.MergeEnvironments(g.Environment, gp.Environment, gv.Environment)
	gopath := env["GOPATH"]
	projectPath := g.RelProjectPath(gopath)

	testOpts := gp.Flags
	if gv.Flags != nil {
		testOpts = testOpts.Merge(gv.Flags)
	}

	relPath := gp.Path
	if relPath != "." && !strings.HasPrefix(relPath, "./") {
		relPath = "./" + relPath
	}
	testOpts = append(testOpts, relPath)

	return &shrub.CmdSubprocessScripting{
		Harness:     "golang",
		WorkingDir:  g.WorkingDirectory,
		HarnessPath: gopath,
		// It is not a problem for the environment to set a relative GOPATH
		// here, which conflicts with the actual GOPATH (an absolute path). The
		// GOPATH in the environment will be overridden when
		// subprocess.scripting runs to be an absolute path relative to the
		// working directory.
		Env:     env,
		TestDir: projectPath,
		TestOptions: &shrub.ScriptingTestOptions{
			Args: testOpts,
		},
	}
}
