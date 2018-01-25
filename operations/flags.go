package operations

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const (
	confFlagName       = "conf"
	adminFlagsFlagName = "flags"
	pathFlagName       = "path"
	projectFlagName    = "project"
	variantsFlagName   = "variants"
	patchIDFlagName    = "patch"
	moduleFlagName     = "module"
	yesFlagName        = "yes"
	tasksFlagName      = "tasks"
	largeFlagName      = "large"

	anserDryRunFlagName  = "dry-run"
	anserLimitFlagName   = "limit"
	anserTargetFlagName  = "target"
	anserWorkersFlagName = "workers"
	anserPeriodFlagName  = "period"
)

func joinFlagNames(ids ...string) string { return strings.Join(ids, ", ") }

func addPathFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(pathFlagName, "filename", "file", "f"),
		Usage: "path to an evergreen project configuration file",
	})
}

func addOutputPath(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(pathFlagName, "filename", "file", "f"),
		Usage: "path to the output file",
	})
}

func serviceConfigFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(confFlagName, "config", "c"),
		Usage: "path to the service configuration file",
		Value: evergreen.DefaultServiceConfigurationFileName,
	})
}

func addProjectFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(projectFlagName, "p"),
		Usage: "specify the name of an existing evergreen project",
	})
}
func addLargeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(largeFlagName, "l"),
		Usage: "enable submitting larger patches (>16MB)",
	})

}

func addTasksFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  joinFlagNames(tasksFlagName, "t"),
		Usage: "task name",
	})
}

func adminFlagFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  adminFlagsFlagName,
		Usage: "specify a flag to disable; may specify more than once",
	})
}

func addVariantsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  joinFlagNames(variantsFlagName, "v"),
		Usage: "variant name(s)",
	})
}

func addPatchIDFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(patchIDFlagName, "id", "i"),
		Usage: "specify the ID of a patch",
	})
}

func addModuleFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(moduleFlagName, "m"),
		Usage: "the name of a module in the project configuration",
	})
}

func addYesFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(yesFlagName, "y"),
		Usage: "skip confirmation text",
	})
}

func addMigrationRuntimeFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags,
		cli.BoolFlag{
			Name:  joinFlagNames(anserDryRunFlagName, "n"),
			Usage: "run migration in a dry-run mode",
		},
		cli.IntFlag{
			Name:  joinFlagNames(anserLimitFlagName, "l"),
			Usage: "limit the number of migration jobs to process",
		},
		cli.IntFlag{
			Name:  joinFlagNames(anserTargetFlagName, "t"),
			Usage: "target number of migrations",
			Value: 60,
		},
		cli.IntFlag{
			Name:  joinFlagNames(anserWorkersFlagName, "j"),
			Usage: "total number of parallel migration workers",
			Value: 4,
		},
		cli.DurationFlag{
			Name:  joinFlagNames(anserPeriodFlagName, "p"),
			Usage: "length of scheduling window",
			Value: time.Minute,
		})

}

func mergeFlagSlices(in ...[]cli.Flag) []cli.Flag {
	out := []cli.Flag{}

	for idx := range in {
		out = append(out, in[idx]...)
	}

	return out
}
