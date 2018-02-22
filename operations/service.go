package operations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/urfave/cli"
)

func Service() cli.Command {
	return cli.Command{
		Name:  "service",
		Usage: "run evergreen services",
		Subcommands: []cli.Command{
			deploy(),
			startRunnerService(),
			startWebService(),
			handcrankRunner(),
		},
	}
}

func deploy() cli.Command {
	return cli.Command{
		Name:  "deploy",
		Usage: "deployment helpers for evergreen site administration",
		Subcommands: []cli.Command{
			deployMigration(),
			deployDataTransforms(),
			smokeStartEvergreen(),
			smokeTestEndpoints(),
			fetchAllProjectConfigs(),
		},
	}
}

func parseDB(c *cli.Context) *evergreen.DBSettings {
	if c == nil {
		return nil
	}
	return &evergreen.DBSettings{
		Url: c.String(dbUrlFlagName),
		SSL: c.Bool(dbSslFlagName),
		DB:  c.String(dbNameFlagName),
		WriteConcernSettings: evergreen.WriteConcern{
			W:     c.Int(dbWriteNumFlagName),
			WMode: c.String(dbWmodeFlagName),
			FSync: c.Bool(dbFsyncFlagName),
			J:     c.Bool(dbJournalAckFlagName),
		},
	}
}
