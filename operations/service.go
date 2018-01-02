package operations

import "github.com/urfave/cli"

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
