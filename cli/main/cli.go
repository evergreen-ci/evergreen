package main

import (
	"github.com/evergreen-ci/evergreen/cli"
	"github.com/jessevdk/go-flags"
	"os"
)

func main() {
	opts := cli.Options{}

	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("set-module", "update or add module to an existing patch", "", &cli.SetModuleCommand{GlobalOpts: opts})
	parser.AddCommand("patch", "submit a patch", "", &cli.PatchCommand{GlobalOpts: opts})
	parser.AddCommand("list-patches", "show existing patches", "", &cli.ListPatchesCommand{GlobalOpts: opts})
	parser.AddCommand("rm-module", "remove a module from an existing patch", "", &cli.RemoveModuleCommand{GlobalOpts: opts})
	parser.AddCommand("cancel-patch", "cancel an existing patch", "", &cli.CancelPatchCommand{GlobalOpts: opts})
	parser.AddCommand("finalize-patch", "finalize an existing patch", "", &cli.FinalizePatchCommand{GlobalOpts: opts})
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
