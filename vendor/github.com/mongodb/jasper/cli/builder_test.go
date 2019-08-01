package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSubcommands(t *testing.T) {
	binary := "binary"

	for _, testCase := range []struct {
		subcommand      []string
		buildSubcommand func(...string) []string
	}{
		{subcommand: []string{binary, JasperCommand}, buildSubcommand: BuildJasperCommand},

		{subcommand: []string{binary, JasperCommand, ServiceCommand}, buildSubcommand: BuildServiceCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, InstallCommand}, buildSubcommand: BuildServiceInstallCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, UninstallCommand}, buildSubcommand: BuildServiceUninstallCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, StartCommand}, buildSubcommand: BuildServiceStartCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, StopCommand}, buildSubcommand: BuildServiceStopCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, RestartCommand}, buildSubcommand: BuildServiceRestartCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, RunCommand}, buildSubcommand: BuildServiceRunCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, StatusCommand}, buildSubcommand: BuildServiceStatusCommand},
		{subcommand: []string{binary, JasperCommand, ServiceCommand, ForceReinstallCommand}, buildSubcommand: BuildServiceForceReinstallCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand}, buildSubcommand: BuildClientCommand},

		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand}, buildSubcommand: BuildManagerCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, IDCommand}, buildSubcommand: BuildManagerIDCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, CreateProcessCommand}, buildSubcommand: BuildManagerCreateProcessCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, CreateCommand}, buildSubcommand: BuildManagerCreateCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, GetCommand}, buildSubcommand: BuildManagerGetCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, GroupCommand}, buildSubcommand: BuildManagerGroupCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, ListCommand}, buildSubcommand: BuildManagerListCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, ClearCommand}, buildSubcommand: BuildManagerClearCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ManagerCommand, CloseCommand}, buildSubcommand: BuildManagerCloseCommand},

		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand}, buildSubcommand: BuildProcessCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, InfoCommand}, buildSubcommand: BuildProcessInfoCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, RunningCommand}, buildSubcommand: BuildProcessRunningCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, CompleteCommand}, buildSubcommand: BuildProcessCompleteCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, TagCommand}, buildSubcommand: BuildProcessTagCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, GetTagsCommand}, buildSubcommand: BuildProcessGetTagsCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, ResetTagsCommand}, buildSubcommand: BuildProcessResetTagsCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, RespawnCommand}, buildSubcommand: BuildProcessRespawnCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, RegisterSignalTriggerIDCommand}, buildSubcommand: BuildProcessRegisterSignalTriggerIDCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, SignalCommand}, buildSubcommand: BuildProcessSignalCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, ProcessCommand, WaitCommand}, buildSubcommand: BuildProcessWaitCommand},

		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand}, buildSubcommand: BuildRemoteCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, ConfigureCacheCommand}, buildSubcommand: BuildRemoteConfigureCacheCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, DownloadFileCommand}, buildSubcommand: BuildRemoteDownloadFileCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, DownloadMongoDBCommand}, buildSubcommand: BuildRemoteDownloadMongoDBCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, GetLogStreamCommand}, buildSubcommand: BuildRemoteGetLogStreamCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, GetBuildloggerURLsCommand}, buildSubcommand: BuildRemoteGetBuildloggerURLsCommand},
		{subcommand: []string{binary, JasperCommand, ClientCommand, RemoteCommand, SignalEventCommand}, buildSubcommand: BuildRemoteSignalEventCommand},
	} {
		t.Run(strings.Join(testCase.subcommand, "/"), func(t *testing.T) {
			assert.Equal(t, testCase.subcommand, testCase.buildSubcommand(binary))
		})
	}
}
