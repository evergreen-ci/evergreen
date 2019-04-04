package operations

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const (
	confFlagName          = "conf"
	overwriteConfFlagName = "overwrite"
	adminFlagsFlagName    = "flags"
	pathFlagName          = "path"
	projectFlagName       = "project"
	variantsFlagName      = "variants"
	patchIDFlagName       = "patch"
	moduleFlagName        = "module"
	yesFlagName           = "yes"
	tasksFlagName         = "tasks"
	largeFlagName         = "large"
	hostFlagName          = "host"
	startTimeFlagName     = "time"
	limitFlagName         = "limit"
	messageFlagName       = "message"
	activeFlagName        = "active"
	committedFlagName     = "committed-only"

	anserDryRunFlagName      = "dry-run"
	anserLimitFlagName       = "limit"
	anserTargetFlagName      = "target"
	anserWorkersFlagName     = "workers"
	anserPeriodFlagName      = "period"
	anserMigrationIDFlagName = "id"

	dbUrlFlagName      = "url"
	dbNameFlagName     = "db"
	dbWriteNumFlagName = "w"
	dbWmodeFlagName    = "wmode"
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
	return append(flags,
		cli.StringFlag{
			Name:  joinFlagNames(confFlagName, "config", "c"),
			Usage: "path to the service configuration file",
		},
		cli.BoolFlag{
			Name:  overwriteConfFlagName,
			Usage: "overwrite the configuration in the db with the file",
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

func addHostFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  hostFlagName,
		Usage: "specify the name of an evergreen host",
	})

}

func addStartTimeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(startTimeFlagName, "t"),
		Usage: "only search for events before this time (RFC 3339 format)",
	})
}

func addLimitFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.IntFlag{
		Name:  joinFlagNames(limitFlagName, "l"),
		Usage: "return a maximum of this number of results",
		Value: 10,
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
		cli.StringSliceFlag{
			Name:  joinFlagNames(anserMigrationIDFlagName, "i"),
			Usage: "Specify one or more times to limit to a specific (named) subset of migrations",
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

func addDbSettingsFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags,
		cli.StringFlag{
			Name:  dbUrlFlagName,
			Usage: "Database URL(s). For a replica set, list all members separated by a comma.",
			Value: evergreen.DefaultDatabaseUrl,
		},
		cli.StringFlag{
			Name:  dbNameFlagName,
			Usage: "Database name",
			Value: evergreen.DefaultDatabaseName,
		},
		cli.IntFlag{
			Name:  dbWriteNumFlagName,
			Usage: "Number of mongod instances that need to acknowledge a write",
		},
		cli.StringFlag{
			Name:  dbWmodeFlagName,
			Usage: "Write mode. Only valid values are blank or 'majority'",
			Value: evergreen.DefaultDatabaseWriteMode,
		},
	)
}

func addCommittedOnlyFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  committedFlagName,
		Usage: "diff with HEAD, ignoring working tree changes",
	})
}

func mergeFlagSlices(in ...[]cli.Flag) []cli.Flag {
	out := []cli.Flag{}

	for idx := range in {
		out = append(out, in[idx]...)
	}

	return out
}
