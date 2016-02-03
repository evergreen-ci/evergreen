package main

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/cli"
	"github.com/jessevdk/go-flags"
	"os"
)

type deprecatedCommand struct{ broken, working string }

func (dc *deprecatedCommand) Execute(_ []string) error {
	return fmt.Errorf(
		"Command `%v` is no longer supported.\nPlease use `%v` instead",
		dc.broken, dc.working)
}

func main() {
	opts := cli.Options{}
	var parser = flags.NewParser(&opts, flags.Default)
	parser.AddCommand("get-update", "fetch the latest version of this binary", "", &cli.GetUpdateCommand{GlobalOpts: &opts})
	parser.AddCommand("version", "display version information", "", &cli.VersionCommand{})
	parser.AddCommand("set-module", "update or add module to an existing patch", "", &cli.SetModuleCommand{GlobalOpts: &opts})
	parser.AddCommand("patch", "submit a patch", "",
		&cli.PatchCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: &opts}})
	parser.AddCommand("patch-file", "submit a patch using a diff file", "",
		&cli.PatchFileCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: &opts}})
	parser.AddCommand("list-patches", "show existing patches", "", &cli.ListPatchesCommand{GlobalOpts: &opts})
	parser.AddCommand("rm-module", "remove a module from an existing patch", "", &cli.RemoveModuleCommand{GlobalOpts: &opts})
	parser.AddCommand("cancel-patch", "cancel an existing patch", "", &cli.CancelPatchCommand{GlobalOpts: &opts})
	parser.AddCommand("finalize-patch", "finalize an existing patch", "", &cli.FinalizePatchCommand{GlobalOpts: &opts})
	parser.AddCommand("list", "list available projects, tasks, or variants", "", &cli.ListCommand{GlobalOpts: &opts})
	parser.AddCommand("validate", "validate a config file", "", &cli.ValidateCommand{GlobalOpts: &opts})
	parser.AddCommand("evaluate", "display a project file's evaluated and expanded form", "", &cli.EvaluateCommand{})
	parser.AddCommand("evaluate-tasks", "deprecated. Use `evaluate --tasks`", "",
		&deprecatedCommand{"evaluate-tasks", "evaluate --tasks"})
	parser.AddCommand("evaluate-variants", "deprecated. Use `evaluate --variants`", "",
		&deprecatedCommand{"evaluate-variants", "evaluate --variants"})
	parser.AddCommand("list-projects", "deprecated. Use `list --projects`", "",
		&deprecatedCommand{"list-projects", "list --projects"})
	parser.AddCommand("list-variants", "deprecated. Use `list --variants`", "",
		&deprecatedCommand{"list-variants", "list --variants"})
	parser.AddCommand("list-tasks", "deprecated. Use `list --tasks`", "",
		&deprecatedCommand{"list-tasks", "list --tasks"})
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
