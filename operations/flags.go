package operations

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const (
	confFlagName              = "conf"
	overwriteConfFlagName     = "overwrite"
	pathFlagName              = "path"
	projectFlagName           = "project"
	patchIDFlagName           = "patch"
	moduleFlagName            = "module"
	localModulesFlagName      = "local_modules"
	skipConfirmFlagName       = "skip_confirm"
	yesFlagName               = "yes"
	variantsFlagName          = "variants"
	regexVariantsFlagName     = "regex_variants"
	tasksFlagName             = "tasks"
	regexTasksFlagName        = "regex_tasks"
	parameterFlagName         = "param"
	patchAliasFlagName        = "alias"
	patchFinalizeFlagName     = "finalize"
	patchBrowseFlagName       = "browse"
	syncBuildVariantsFlagName = "sync_variants"
	syncTasksFlagName         = "sync_tasks"
	syncTimeoutFlagName       = "sync_timeout"
	syncStatusesFlagName      = "sync_statuses"
	largeFlagName             = "large"
	hostFlagName              = "host"
	displayNameFlagName       = "name"
	regionFlagName            = "region"
	startTimeFlagName         = "time"
	limitFlagName             = "limit"
	messageFlagName           = "message"
	forceFlagName             = "force"
	activeFlagName            = "active"
	refFlagName               = "ref"
	quietFlagName             = "quiet"
	longFlagName              = "long"
	dirFlagName               = "dir"
	uncommittedChangesFlag    = "uncommitted"
	preserveCommitsFlag       = "preserve-commits"
	subscriptionTypeFlag      = "subscription-type"
	errorOnWarningsFlagName   = "error-on-warnings"

	anserDryRunFlagName      = "dry-run"
	anserLimitFlagName       = "limit"
	anserTargetFlagName      = "target"
	anserWorkersFlagName     = "workers"
	anserPeriodFlagName      = "period"
	anserMigrationIDFlagName = "id"

	dbUrlFlagName       = "url"
	dbCredsFileFlagName = "auth-file"
	dbNameFlagName      = "db"
	dbWriteNumFlagName  = "w"
	dbWmodeFlagName     = "wmode"
	dbRmodeFlagName     = "rmode"

	jsonFlagName = "json"
)

func joinFlagNames(ids ...string) string { return strings.Join(ids, ", ") }

func addPathFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(pathFlagName, "filename", "file", "f"),
		Usage: "path to an Evergreen project configuration file",
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
			Usage: "overwrite the configuration in the DB with the file",
		})
}

func addProjectFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(projectFlagName, "p"),
		Usage: "specify the name of an existing Evergreen project",
	})
}

func addLargeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(largeFlagName, "l"),
		Usage: "enable submitting larger patches (>16MB)",
	})

}

func addParameterFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  parameterFlagName,
		Usage: "specify a parameter as a KEY=VALUE pair",
	})
}

func addPatchFinalizeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(patchFinalizeFlagName, "f"),
		Usage: "schedule tasks immediately",
	})
}

func addPatchBrowseFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(patchBrowseFlagName),
		Usage: "open patch URL in browser",
	})
}

func addSyncBuildVariantsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  syncBuildVariantsFlagName,
		Usage: "build variants to sync when task ends",
	})
}

func addSyncTasksFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  syncTasksFlagName,
		Usage: "tasks to sync when task ends",
	})
}

func addSyncTimeoutFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.DurationFlag{
		Name:  syncTimeoutFlagName,
		Usage: "max timeout on task sync when task ends (e.g. 15m, 2h)",
		Value: evergreen.DefaultTaskSyncAtEndTimeout,
	})
}

func addSyncStatusesFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  syncStatusesFlagName,
		Usage: "filter task statuses for which task sync should run when task ends ('success' or 'failed')",
	})
}

func addVariantsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringSliceFlag{
		Name:  joinFlagNames(variantsFlagName, "v"),
		Usage: "variant names (\"all\" for all variants)",
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

func addSkipConfirmFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(skipConfirmFlagName, yesFlagName, "y"),
		Usage: "skip confirmation text",
	})
}

func addHostFlag(flags ...cli.Flag) []cli.Flag {
	return append([]cli.Flag{
		cli.StringFlag{
			Name:  hostFlagName,
			Usage: "specify `ID` of an evergreen host",
		},
	}, flags...)

}

func addSubscriptionTypeFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  subscriptionTypeFlag,
		Usage: "subscribe for notifications through `TYPE`",
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
			Value: evergreen.DefaultDatabaseURL,
		},
		cli.StringFlag{
			Name:   dbCredsFileFlagName,
			Usage:  "specify a DB credential file location",
			EnvVar: evergreen.MongodbAuthFile,
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
		cli.StringFlag{
			Name:  dbRmodeFlagName,
			Usage: "Read mode. Only valid values are blank or 'majority'",
			Value: evergreen.DefaultDatabaseReadMode,
		},
	)
}

func addRefFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  refFlagName,
		Usage: "diff with `REF`",
		Value: "HEAD",
	})
}

func addCommitsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.StringFlag{
		Name:  joinFlagNames(commitsFlagName, "c"),
		Usage: "specify commit hash <hash1> (can also be a range <hash1>..<hash2>, where hash1 is excluded)",
	})
}

func addUncommittedChangesFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(uncommittedChangesFlag, "u"),
		Usage: "include uncommitted changes",
	})
}

func addReuseFlags(flags ...cli.Flag) []cli.Flag {
	message := "repeat %s: use the %s tasks/variants defined for the %s patch"
	res := append(flags, cli.BoolFlag{
		Name:  joinFlagNames(repeatDefinitionFlag, "reuse"),
		Usage: fmt.Sprintf(message, "latest", "same", "latest"),
	})
	res = append(res, cli.BoolFlag{
		Name:  joinFlagNames(repeatFailedDefinitionFlag, "rf"),
		Usage: fmt.Sprintf(message, "latest failed", "failed", "latest"),
	})
	res = append(res, cli.StringFlag{
		Name:  joinFlagNames(repeatPatchIdFlag, "reuse-patch"),
		Usage: fmt.Sprintf(message, "specific patch", "same", "given"),
	})
	return res
}

func addPreserveCommitsFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  preserveCommitsFlag,
		Usage: "preserve separate commits when enqueueing to the commit queue",
	})
}

func mergeFlagSlices(in ...[]cli.Flag) []cli.Flag {
	out := []cli.Flag{}

	for idx := range in {
		out = append(out, in[idx]...)
	}

	return out
}
