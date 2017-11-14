package main

import (
	"os"

	"github.com/evergreen-ci/evergreen/cli"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	flags "github.com/jessevdk/go-flags"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
)

func init() {
	sender := grip.GetSender()
	l := sender.Level()
	l.Threshold = level.Info
	grip.CatchEmergencyPanic(sender.SetLevel(l))
}

func main() {
	opts := &cli.Options{}
	parser := flags.NewParser(opts, flags.Default)

	parser.AddCommand("get-update", "fetch the latest version of this binary", "", &cli.GetUpdateCommand{GlobalOpts: opts})
	parser.AddCommand("version", "display version information", "", &cli.VersionCommand{})
	parser.AddCommand("set-module", "update or add module to an existing patch", "", &cli.SetModuleCommand{GlobalOpts: opts})
	parser.AddCommand("patch", "submit a patch", "", &cli.PatchCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: opts}})
	parser.AddCommand("patch-file", "submit a patch using a diff file", "", &cli.PatchFileCommand{PatchCommandParams: cli.PatchCommandParams{GlobalOpts: opts}})
	parser.AddCommand("list-patches", "show existing patches", "", &cli.ListPatchesCommand{GlobalOpts: opts})
	parser.AddCommand("rm-module", "remove a module from an existing patch", "", &cli.RemoveModuleCommand{GlobalOpts: opts})
	parser.AddCommand("cancel-patch", "cancel an existing patch", "", &cli.CancelPatchCommand{GlobalOpts: opts})
	parser.AddCommand("finalize-patch", "finalize an existing patch", "", &cli.FinalizePatchCommand{GlobalOpts: opts})
	parser.AddCommand("list", "list available projects, tasks, or variants", "", &cli.ListCommand{GlobalOpts: opts})
	parser.AddCommand("last-green", "return a project's most recent successful version for given variants", "", &cli.LastGreenCommand{GlobalOpts: opts})
	parser.AddCommand("validate", "validate a config file", "", &cli.ValidateCommand{GlobalOpts: opts})
	parser.AddCommand("evaluate", "display a project file's evaluated and expanded form", "", &cli.EvaluateCommand{})
	parser.AddCommand("fetch", "fetch data associated with a task", "", &cli.FetchCommand{GlobalOpts: opts})
	parser.AddCommand("export", "export statistics as csv or json for given options", "", &cli.ExportCommand{GlobalOpts: opts})
	parser.AddCommand("test-history", "retrieve test history for a given project", "", &cli.TestHistoryCommand{GlobalOpts: opts})
	parser.AddCommand("agent", "runs an evergreen agent", "", &cli.AgentCommand{})
	parser.AddCommand("keys", "manage your public keys", "", &cli.PublicKeyCommand{GlobalOpts: opts})

	host, _ := parser.AddCommand("host", "host-related commands", "", &struct{}{})
	host.AddCommand("create", "spawn a host", "", &cli.HostCreateCommand{GlobalOpts: opts})
	host.AddCommand("list", "list hosts", "", &cli.HostListCommand{GlobalOpts: opts})
	host.AddCommand("terminate", "terminate a host", "", &cli.HostTerminateCommand{GlobalOpts: opts})
	host.AddCommand("status", "return the status of a host", "", &cli.HostStatusCommand{GlobalOpts: opts})
	host.AddCommand("setup", "run a setup script on a host", "", &cli.HostSetupCommand{})
	host.AddCommand("teardown", "run a teardown script on a host", "", &cli.HostTeardownCommand{})

	service, _ := parser.AddCommand("service", "run evergreen services", "", &struct{}{})
	service.AddCommand("runner", "start background task processing", "", &cli.ServiceRunnerCommand{})
	service.AddCommand("web", "start web tier services for API and UI", "", &cli.ServiceWebCommand{})

	admin, _ := parser.AddCommand("admin", "administer evergreen deployment", "", &struct{}{})
	admin.AddCommand("banner", "modify content of site-wide display banner", "", &cli.AdminBannerCommand{GlobalOpts: opts})
	admin.AddCommand("disable-service", "disable component services", "", &cli.AdminDisableServiceCommand{GlobalOpts: opts})
	admin.AddCommand("enable-service", "enable component services", "", &cli.AdminEnableServiceCommand{GlobalOpts: opts})

	deploy, _ := service.AddCommand("deploy", "deployment helper (e.g. migration tools)", "", &struct{}{})
	deploy.AddCommand("anser", "migration helper", "", &cli.MigrationCommand{})

	// run commands
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}
