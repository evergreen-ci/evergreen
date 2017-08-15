package main

import (
	"strings"

	"github.com/evergreen-ci/evergreen/cli"
	flags "github.com/jessevdk/go-flags"
	"github.com/mongodb/grip"
)

func main() {
	opts := cli.Options{}
	parser := flags.NewParser(&opts, flags.Default)

	parser.AddCommand("get-update", "fetch the latest version of this binary", "", &cli.GetUpdateCommand{GlobalOpts: &opts})
	parser.AddCommand("version", "display version information", "", &cli.VersionCommand{})
	parser.AddCommand("set-module", "update or add module to an existing patch", "", &cli.SetModuleCommand{GlobalOpts: &opts})
	parser.AddCommand("patch", "submit a patch", "", &cli.PatchCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: &opts}})
	parser.AddCommand("patch-file", "submit a patch using a diff file", "", &cli.PatchFileCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: &opts}})
	parser.AddCommand("list-patches", "show existing patches", "", &cli.ListPatchesCommand{GlobalOpts: &opts})
	parser.AddCommand("rm-module", "remove a module from an existing patch", "", &cli.RemoveModuleCommand{GlobalOpts: &opts})
	parser.AddCommand("cancel-patch", "cancel an existing patch", "", &cli.CancelPatchCommand{GlobalOpts: &opts})
	parser.AddCommand("finalize-patch", "finalize an existing patch", "", &cli.FinalizePatchCommand{GlobalOpts: &opts})
	parser.AddCommand("list", "list available projects, tasks, or variants", "", &cli.ListCommand{GlobalOpts: &opts})
	parser.AddCommand("last-green", "return a project's most recent successful version for given variants", "", &cli.LastGreenCommand{GlobalOpts: &opts})
	parser.AddCommand("validate", "validate a config file", "", &cli.ValidateCommand{GlobalOpts: &opts})
	parser.AddCommand("evaluate", "display a project file's evaluated and expanded form", "", &cli.EvaluateCommand{})
	parser.AddCommand("fetch", "fetch data associated with a task", "", &cli.FetchCommand{GlobalOpts: &opts})
	parser.AddCommand("export", "export statistics as csv or json for given options", "", &cli.ExportCommand{GlobalOpts: &opts})
	parser.AddCommand("test-history", "retrieve test history for a given project", "", &cli.TestHistoryCommand{GlobalOpts: &opts})
	parser.AddCommand("agent", "runs an evergreen agent", "", &cli.AgentCommand{})
	hosts := &cli.HostCommand{}
	cmd, _ := parser.AddCommand("host", "host-related commands", "", hosts)
	cmd.AddCommand("create", "spawn a host", "", hosts.GetSubCommand("create", &opts))
	cmd.AddCommand("list", "list hosts", "", hosts.GetSubCommand("list", &opts))
	cmd.AddCommand("terminate", "terminate a host", "", hosts.GetSubCommand("terminate", &opts))
	cmd.AddCommand("status", "return the status of a host", "", hosts.GetSubCommand("status", &opts))

	args, err := parser.Parse()
	if len(args) > 0 && strings.Contains(args[0], "help") {
		return
	}

	grip.CatchEmergencyFatal(err)
}
