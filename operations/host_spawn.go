package operations

import "github.com/urfave/cli"

func hostCreate() cli.Command {
	return cli.Command{
		Name:  "create",
		Usage: "spawn a host",
		Flags: clientConfigFlags(),
	}

}
func hostlist() cli.Command      {}
func hostTerminate() cli.Command {}
func hostStatus() cli.Command    {}
