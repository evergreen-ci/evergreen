package operations

import (
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
)

func addPathFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    pathFlagName,
		Aliases: []string{"filename", "file", "f"},
		Usage:   "path to an evergreen project configuration file",
	})
}

func addOutputPath(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    pathFlagName,
		Aliases: []string{"filename", "file", "f"},
		Usage:   "path to the output file",
	})
}

func serviceConfigFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    confFlagName,
		Aliases: []string{"c", "config"},
		Usage:   "path to the service configuration file",
		Default: evergreen.DefaultServiceConfigurationFileName,
	})
}

func addProjectFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    projectFlagName,
		Aliases: []string{"p"},
		Usage:   "specify the name of an existing evergreen project",
	})
}
func addLargeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  largeFlagName,
		Usage: "enable submitting larger patches (>16MB)",
	})

}

func addTasksFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  taskFlagName,
		Usage: "task name",
	})
}

func adminFlagFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  flagFlagName,
		Usage: "specify a flag to disable; may specify more than once",
	})
}

func addVariantsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:    variantsFlagName,
		Aliases: []string{"v"},
		Usage:   "variant name(s)",
	})
}

func addPatchIDFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    patchIDFlagName,
		Aliases: []string{"i", "id", "patch"},
		Usage:   "specify the ID of a patch",
	})
}

func addModuleFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:    moduleFlagName,
		Aliases: []string{"m"},
		Usage:   "the name of a module in the project configuration",
	})
}

func addYesFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:    yesFlagName,
		Aliases: []string{"y"},
		Usage:   "skip confirmation text",
	})
}

func mergeFlagSlices(in ...[]cli.Flag) []cli.Flag {
	out := []cli.Flag{}

	for idx := range in {
		out = append(out, in[idx]...)
	}

	return out
}
