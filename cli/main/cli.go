package main

import (
	"github.com/evergreen-ci/evergreen/cli"
	"github.com/jessevdk/go-flags"
	"os"
)

func main() {
	opts := cli.Options{}

	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("get-update", "fetch the latest version of this binary", "", &cli.GetUpdateCommand{GlobalOpts: opts})
	parser.AddCommand("version", "display version information", "", &cli.VersionCommand{})
	parser.AddCommand("set-module", "update or add module to an existing patch", "", &cli.SetModuleCommand{GlobalOpts: opts})
	parser.AddCommand("patch", "submit a patch", "", &cli.PatchCommand{GlobalOpts: opts})
	parser.AddCommand("list-patches", "show existing patches", "", &cli.ListPatchesCommand{GlobalOpts: opts})
	parser.AddCommand("rm-module", "remove a module from an existing patch", "", &cli.RemoveModuleCommand{GlobalOpts: opts})
	parser.AddCommand("cancel-patch", "cancel an existing patch", "", &cli.CancelPatchCommand{GlobalOpts: opts})
	parser.AddCommand("finalize-patch", "finalize an existing patch", "", &cli.FinalizePatchCommand{GlobalOpts: opts})
	parser.AddCommand("list-projects", "list all projects", "", &cli.ListProjectsCommand{GlobalOpts: opts})
	parser.AddCommand("validate", "validate a config file", "", &cli.ValidateCommand{GlobalOpts: opts})
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
