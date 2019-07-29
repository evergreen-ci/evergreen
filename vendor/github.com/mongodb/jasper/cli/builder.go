package cli

// This file contains convenience functions to build the string versions of CLI
// subcommands.

// BuildJasperCommand is a convenience function to generate the slice of strings
// to invoke the Jasper subcommand.
func BuildJasperCommand(basePrefix ...string) []string {
	return append(append([]string{}, basePrefix...), JasperCommand)
}

// Jasper.Service builders

// BuildServiceCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Service subcommand.
func BuildServiceCommand(basePrefix ...string) []string {
	return append(BuildJasperCommand(basePrefix...), ServiceCommand)
}

// BuildServiceInstallCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Service.Install subcommand.
func BuildServiceInstallCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), InstallCommand)
}

// BuildServiceUninstallCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Uninstall subcommand.
func BuildServiceUninstallCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), UninstallCommand)
}

// BuildServiceStartCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Start subcommand.
func BuildServiceStartCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), StartCommand)
}

// BuildServiceStopCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Stop subcommand.
func BuildServiceStopCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), StopCommand)
}

// BuildServiceRestartCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Restart subcommand.
func BuildServiceRestartCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), RestartCommand)
}

// BuildServiceRunCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Run subcommand.
func BuildServiceRunCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), RunCommand)
}

// BuildServiceStatusCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Service.Status subcommand.
func BuildServiceStatusCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), StatusCommand)
}

// BuildServiceForceReinstallCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Service.ForceReinstall subcommand.
func BuildServiceForceReinstallCommand(basePrefix ...string) []string {
	return append(BuildServiceCommand(basePrefix...), ForceReinstallCommand)
}

// Jasper.Client builders

// BuildClientCommand is a convenience function to generate the slice of strings
// to invoke the Client subcommand.
func BuildClientCommand(basePrefix ...string) []string {
	return append(BuildJasperCommand(basePrefix...), ClientCommand)
}

// Jasper.Client.Manager builders

// BuildManagerCommand is a convenience function to generate the slice of strings
// to invoke the Jasper.Client.Manager subcommand.
func BuildManagerCommand(basePrefix ...string) []string {
	return append(BuildClientCommand(basePrefix...), ManagerCommand)
}

// BuildManagerCreateProcessCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.CreateProcess
// subcommand.
func BuildManagerCreateProcessCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), CreateProcessCommand)
}

// BuildManagerCreateCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.CreateCommand
// subcommand.
func BuildManagerCreateCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), CreateCommand)
}

// BuildManagerGetCommand is a convenience function to generate the slice
// of strings to invoke the Jasper.Client.Manager.Get subcommand.
func BuildManagerGetCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), GetCommand)
}

// BuildManagerGroupCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.Group
// subcommand.
func BuildManagerGroupCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), GroupCommand)
}

// BuildManagerListCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.List
// subcommand.
func BuildManagerListCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), ListCommand)
}

// BuildManagerClearCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.Clear
// subcommand.
func BuildManagerClearCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), ClearCommand)
}

// BuildManagerCloseCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Manager.Close
// subcommand.
func BuildManagerCloseCommand(basePrefix ...string) []string {
	return append(BuildManagerCommand(basePrefix...), CloseCommand)
}

// Jasper.Client.Process builders

// BuildProcessCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process subcommand.
func BuildProcessCommand(basePrefix ...string) []string {
	return append(BuildClientCommand(basePrefix...), ProcessCommand)
}

// BuildProcessInfoCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Info subcommand.
func BuildProcessInfoCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), InfoCommand)
}

// BuildProcessRunningCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Running subcommand.
func BuildProcessRunningCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), RunningCommand)
}

// BuildProcessCompleteCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Complete subcommand.
func BuildProcessCompleteCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), CompleteCommand)
}

// BuildProcessTagCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Tag subcommand.
func BuildProcessTagCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), TagCommand)
}

// BuildProcessGetTagsCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.GetTags subcommand.
func BuildProcessGetTagsCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), GetTagsCommand)
}

// BuildProcessResetTagsCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.ResetTags subcommand.
func BuildProcessResetTagsCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), ResetTagsCommand)
}

// BuildProcessRespawnCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Respawn subcommand.
func BuildProcessRespawnCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), RespawnCommand)
}

// BuildProcessRegisterSignalTriggerIDCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.RegisterSignalTriggerID subcommand.
func BuildProcessRegisterSignalTriggerIDCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), RegisterSignalTriggerIDCommand)
}

// BuildProcessSignalCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Signal subcommand.
func BuildProcessSignalCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), SignalCommand)
}

// BuildProcessWaitCommand is a convenience function to generate the slice of
// strings to invoke the Jasper.Client.Process.Wait subcommand.
func BuildProcessWaitCommand(basePrefix ...string) []string {
	return append(BuildProcessCommand(basePrefix...), WaitCommand)
}

// Jasper.Client.Remote builders

// BuildRemoteCommand is a convenience function to generate the slice of strings
// to invoke the Jasper.Client.Remote subcommand.
func BuildRemoteCommand(basePrefix ...string) []string {
	return append(BuildClientCommand(basePrefix...), RemoteCommand)
}

// BuildRemoteConfigureCacheCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.ConfigureCache
// subcommand.
func BuildRemoteConfigureCacheCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), ConfigureCacheCommand)
}

// BuildRemoteDownloadFileCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.DownloadFile
// subcommand.
func BuildRemoteDownloadFileCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), DownloadFileCommand)
}

// BuildRemoteDownloadMongoDBCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.DownloadMongoDB
// subcommand.
func BuildRemoteDownloadMongoDBCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), DownloadMongoDBCommand)
}

// BuildRemoteGetLogStreamCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.GetLogStream
// subcommand.
func BuildRemoteGetLogStreamCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), GetLogStreamCommand)
}

// BuildRemoteGetBuildloggerURLsCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.GetBuildloggerURLs
// subcommand.
func BuildRemoteGetBuildloggerURLsCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), GetBuildloggerURLsCommand)
}

// BuildRemoteSignalEventCommand is a convenience function to generate the
// slice of strings to invoke the Jasper.Client.Remote.SignalEvent
// subcommand.
func BuildRemoteSignalEventCommand(basePrefix ...string) []string {
	return append(BuildRemoteCommand(basePrefix...), SignalEventCommand)
}
