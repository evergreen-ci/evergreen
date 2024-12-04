package operations

import (
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

const (
	clientS3BucketFlagName    = "client_s3_bucket"
	confFlagName              = "conf"
	dirFlagName               = "dir"
	displayNameFlagName       = "name"
	errorOnWarningsFlagName   = "error-on-warnings"
	forceFlagName             = "force"
	hostFlagName              = "host"
	largeFlagName             = "large"
	limitFlagName             = "limit"
	localModulesFlagName      = "local_modules"
	longFlagName              = "long"
	moduleFlagName            = "module"
	overwriteConfFlagName     = "overwrite"
	parameterFlagName         = "param"
	patchAliasFlagName        = "alias"
	patchAuthorFlag           = "author"
	patchBrowseFlagName       = "browse"
	patchFinalizeFlagName     = "finalize"
	patchIDFlagName           = "patch"
	pathFlagName              = "path"
	preserveCommitsFlag       = "preserve-commits"
	projectFlagName           = "project"
	quietFlagName             = "quiet"
	refFlagName               = "ref"
	regexTasksFlagName        = "regex_tasks"
	regexVariantsFlagName     = "regex_variants"
	regionFlagName            = "region"
	skipConfirmFlagName       = "skip_confirm"
	startTimeFlagName         = "time"
	subscriptionTypeFlag      = "subscription-type"
	syncBuildVariantsFlagName = "sync_variants"
	syncStatusesFlagName      = "sync_statuses"
	syncTasksFlagName         = "sync_tasks"
	syncTimeoutFlagName       = "sync_timeout"
	tasksFlagName             = "tasks"
	traceEndpointFlagName     = "trace_endpoint"
	uncommittedChangesFlag    = "uncommitted"
	variantsFlagName          = "variants"
	versionIDFlagName         = "version_id"
	yesFlagName               = "yes"

	dbAWSAuthFlagName   = "mongo-aws-auth"
	dbNameFlagName      = "db"
	dbRmodeFlagName     = "rmode"
	dbUrlFlagName       = "url"
	dbWmodeFlagName     = "wmode"
	dbWriteNumFlagName  = "w"
	sharedDBUrlFlagName = "shared-db-url"

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
		cli.StringFlag{
			Name:   versionIDFlagName,
			Usage:  "version ID of the client build to link to",
			EnvVar: evergreen.EvergreenVersionID,
		},
		cli.StringFlag{
			Name:   clientS3BucketFlagName,
			Usage:  "S3 bucket where the Evergreen clients are located",
			EnvVar: evergreen.EvergreenClientS3Bucket,
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

func addDbSettingsFlags(flags ...cli.Flag) []cli.Flag {
	return append(flags,
		cli.StringFlag{
			Name:  dbUrlFlagName,
			Usage: "Database URL(s). For a replica set, list all members separated by a comma.",
			Value: evergreen.DefaultDatabaseURL,
		},
		cli.StringFlag{
			Name:  sharedDBUrlFlagName,
			Usage: "Database URL(s) for the shared database. For a replica set, list all members separated by a comma.",
		},
		cli.BoolFlag{
			Name:   dbAWSAuthFlagName,
			Usage:  "Enable MONGODB_AWS authentication with the database.",
			EnvVar: evergreen.MongoAWSAuthEnabled,
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

func addUncommittedChangesFlag(flags ...cli.Flag) []cli.Flag {
	return append(flags, cli.BoolFlag{
		Name:  joinFlagNames(uncommittedChangesFlag, "u"),
		Usage: "include uncommitted changes",
	})
}

func addReuseFlags(flags ...cli.Flag) []cli.Flag {
	message := "repeat %s: use the %s tasks/variants defined for the %s patch %s"
	res := append(flags, cli.BoolFlag{
		Name:  joinFlagNames(repeatDefinitionFlag, "reuse"),
		Usage: fmt.Sprintf(message, "latest", "same", "latest", ""),
	})
	res = append(res, cli.BoolFlag{
		Name:  joinFlagNames(repeatFailedDefinitionFlag, "rf"),
		Usage: fmt.Sprintf(message, "latest failed", "failed", "latest", "(can be overridden by the --reuse-patch flag)"),
	})
	res = append(res, cli.StringFlag{
		Name:  joinFlagNames(repeatPatchIdFlag, "reuse-patch"),
		Usage: fmt.Sprintf(message, "specific patch", "same", "given", ""),
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
