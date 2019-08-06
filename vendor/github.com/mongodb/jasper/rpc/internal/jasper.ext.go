package internal

import (
	"bytes"
	"syscall"
	"time"

	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/tychoish/bond"
)

// Export takes a protobuf RPC CreateOptions struct and returns the analogous
// Jasper CreateOptions struct. It is not safe to concurrently access the
// exported RPC CreateOptions and the returned Jasper CreateOptions.
func (opts *CreateOptions) Export() *jasper.CreateOptions {
	out := &jasper.CreateOptions{
		Args:               opts.Args,
		Environment:        opts.Environment,
		WorkingDirectory:   opts.WorkingDirectory,
		Timeout:            time.Duration(opts.TimeoutSeconds) * time.Second,
		TimeoutSecs:        int(opts.TimeoutSeconds),
		OverrideEnviron:    opts.OverrideEnviron,
		Tags:               opts.Tags,
		StandardInputBytes: opts.StandardInputBytes,
	}
	if len(opts.StandardInputBytes) != 0 {
		out.StandardInput = bytes.NewBuffer(opts.StandardInputBytes)
	}

	if opts.Output != nil {
		out.Output = opts.Output.Export()
	}

	for _, opt := range opts.OnSuccess {
		out.OnSuccess = append(out.OnSuccess, opt.Export())
	}
	for _, opt := range opts.OnFailure {
		out.OnFailure = append(out.OnFailure, opt.Export())
	}
	for _, opt := range opts.OnTimeout {
		out.OnTimeout = append(out.OnTimeout, opt.Export())
	}

	return out
}

// ConvertCreateOptions takes a Jasper CreateOptions struct and returns an
// equivalent protobuf RPC *CreateOptions struct. ConvertCreateOptions is the
// inverse of (*CreateOptions) Export(). It is not safe to concurrently
// access the converted Jasper CreateOptions and the returned RPC
// CreateOptions.
func ConvertCreateOptions(opts *jasper.CreateOptions) *CreateOptions {
	if opts.TimeoutSecs == 0 && opts.Timeout != 0 {
		opts.TimeoutSecs = int(opts.Timeout.Seconds())
	}

	output := ConvertOutputOptions(opts.Output)

	co := &CreateOptions{
		Args:               opts.Args,
		Environment:        opts.Environment,
		WorkingDirectory:   opts.WorkingDirectory,
		TimeoutSeconds:     int64(opts.TimeoutSecs),
		OverrideEnviron:    opts.OverrideEnviron,
		Tags:               opts.Tags,
		Output:             &output,
		StandardInputBytes: opts.StandardInputBytes,
	}

	for _, opt := range opts.OnSuccess {
		co.OnSuccess = append(co.OnSuccess, ConvertCreateOptions(opt))
	}
	for _, opt := range opts.OnFailure {
		co.OnFailure = append(co.OnFailure, ConvertCreateOptions(opt))
	}
	for _, opt := range opts.OnTimeout {
		co.OnTimeout = append(co.OnTimeout, ConvertCreateOptions(opt))
	}

	return co
}

// Export takes a protobuf RPC ProcessInfo struct and returns the analogous
// Jasper ProcessInfo struct.
func (info *ProcessInfo) Export() jasper.ProcessInfo {
	return jasper.ProcessInfo{
		ID:         info.Id,
		PID:        int(info.Pid),
		IsRunning:  info.Running,
		Successful: info.Successful,
		Complete:   info.Complete,
		ExitCode:   int(info.ExitCode),
		Timeout:    info.Timedout,
		Options:    *info.Options.Export(),
	}
}

// ConvertProcessInfo takes a Jasper ProcessInfo struct and returns an
// equivalent protobuf RPC *ProcessInfo struct. ConvertProcessInfo is the
// inverse of (*ProcessInfo) Export().
func ConvertProcessInfo(info jasper.ProcessInfo) *ProcessInfo {
	return &ProcessInfo{
		Id:         info.ID,
		Pid:        int64(info.PID),
		ExitCode:   int32(info.ExitCode),
		Running:    info.IsRunning,
		Successful: info.Successful,
		Complete:   info.Complete,
		Timedout:   info.Timeout,
		Options:    ConvertCreateOptions(&info.Options),
	}
}

// Export takes a protobuf RPC Signals struct and returns the analogous
// syscall.Signal.
func (s Signals) Export() syscall.Signal {
	switch s {
	case Signals_HANGUP:
		return syscall.SIGHUP
	case Signals_INIT:
		return syscall.SIGINT
	case Signals_TERMINATE:
		return syscall.SIGTERM
	case Signals_KILL:
		return syscall.SIGKILL
	default:
		return syscall.Signal(0)
	}
}

// ConvertSignal takes a syscall.Signal and returns an
// equivalent protobuf RPC Signals struct. ConvertSignals is the
// inverse of (Signals) Export().
func ConvertSignal(s syscall.Signal) Signals {
	switch s {
	case syscall.SIGHUP:
		return Signals_HANGUP
	case syscall.SIGINT:
		return Signals_INIT
	case syscall.SIGTERM:
		return Signals_TERMINATE
	case syscall.SIGKILL:
		return Signals_KILL
	default:
		return Signals_UNKNOWN
	}
}

// ConvertFilter takes a Jasper Filter struct and returns an
// equivalent protobuf RPC *Filter struct.
func ConvertFilter(f jasper.Filter) *Filter {
	switch f {
	case jasper.All:
		return &Filter{Name: FilterSpecifications_ALL}
	case jasper.Running:
		return &Filter{Name: FilterSpecifications_RUNNING}
	case jasper.Terminated:
		return &Filter{Name: FilterSpecifications_TERMINATED}
	case jasper.Failed:
		return &Filter{Name: FilterSpecifications_FAILED}
	case jasper.Successful:
		return &Filter{Name: FilterSpecifications_SUCCESSFUL}
	default:
		return nil
	}
}

// Export takes a protobuf RPC LogType struct and returns the analogous
// Jasper LogType struct.
func (lt LogType) Export() jasper.LogType {
	switch lt {
	case LogType_LOGBUILDLOGGERV2:
		return jasper.LogBuildloggerV2
	case LogType_LOGBUILDLOGGERV3:
		return jasper.LogBuildloggerV3
	case LogType_LOGDEFAULT:
		return jasper.LogDefault
	case LogType_LOGFILE:
		return jasper.LogFile
	case LogType_LOGINHERIT:
		return jasper.LogInherit
	case LogType_LOGSPLUNK:
		return jasper.LogSplunk
	case LogType_LOGSUMOLOGIC:
		return jasper.LogSumologic
	case LogType_LOGINMEMORY:
		return jasper.LogInMemory
	default:
		return jasper.LogType("")
	}
}

// ConvertLogType takes a Jasper LogType struct and returns an
// equivalent protobuf RPC LogType struct. ConvertLogType is the
// inverse of (LogType) Export().
func ConvertLogType(lt jasper.LogType) LogType {
	switch lt {
	case jasper.LogBuildloggerV2:
		return LogType_LOGBUILDLOGGERV2
	case jasper.LogBuildloggerV3:
		return LogType_LOGBUILDLOGGERV3
	case jasper.LogDefault:
		return LogType_LOGDEFAULT
	case jasper.LogFile:
		return LogType_LOGFILE
	case jasper.LogInherit:
		return LogType_LOGINHERIT
	case jasper.LogSplunk:
		return LogType_LOGSPLUNK
	case jasper.LogSumologic:
		return LogType_LOGSUMOLOGIC
	case jasper.LogInMemory:
		return LogType_LOGINMEMORY
	default:
		return LogType_LOGUNKNOWN
	}
}

// Export takes a protobuf RPC OutputOptions struct and returns the analogous
// Jasper OutputOptions struct.
func (opts OutputOptions) Export() jasper.OutputOptions {
	loggers := []jasper.Logger{}
	for _, logger := range opts.Loggers {
		loggers = append(loggers, logger.Export())
	}
	return jasper.OutputOptions{
		SuppressOutput:    opts.SuppressOutput,
		SuppressError:     opts.SuppressError,
		SendOutputToError: opts.RedirectOutputToError,
		SendErrorToOutput: opts.RedirectErrorToOutput,
		Loggers:           loggers,
	}
}

// ConvertOutputOptions takes a Jasper OutputOptions struct and returns an
// equivalent protobuf RPC OutputOptions struct. ConvertOutputOptions is the
// inverse of (OutputOptions) Export().
func ConvertOutputOptions(opts jasper.OutputOptions) OutputOptions {
	loggers := []*Logger{}
	for _, logger := range opts.Loggers {
		loggers = append(loggers, ConvertLogger(logger))
	}
	return OutputOptions{
		SuppressOutput:        opts.SuppressOutput,
		SuppressError:         opts.SuppressError,
		RedirectOutputToError: opts.SendOutputToError,
		RedirectErrorToOutput: opts.SendErrorToOutput,
		Loggers:               loggers,
	}
}

// Export takes a protobuf RPC Logger struct and returns the analogous
// Jasper Logger struct.
func (logger Logger) Export() jasper.Logger {
	return jasper.Logger{
		Type:    logger.LogType.Export(),
		Options: logger.LogOptions.Export(),
	}
}

// ConvertLogger takes a Jasper Logger struct and returns an
// equivalent protobuf RPC Logger struct. ConvertLogger is the
// inverse of (Logger) Export().
func ConvertLogger(logger jasper.Logger) *Logger {
	return &Logger{
		LogType:    ConvertLogType(logger.Type),
		LogOptions: ConvertLogOptions(logger.Options),
	}
}

// Export takes a protobuf RPC LogOptions struct and returns the analogous
// Jasper LogOptions struct.
func (opts LogOptions) Export() jasper.LogOptions {
	out := jasper.LogOptions{
		DefaultPrefix: opts.DefaultPrefix,
		FileName:      opts.FileName,
		Format:        opts.Format.Export(),
		InMemoryCap:   int(opts.InMemoryCap),
		SumoEndpoint:  opts.SumoEndpoint,
	}

	if opts.SplunkOptions != nil {
		out.SplunkOptions = opts.SplunkOptions.Export()
	}
	if opts.BufferOptions != nil {
		out.BufferOptions = opts.BufferOptions.Export()
	}
	if opts.BuildloggerOptions != nil {
		out.BuildloggerOptions = opts.BuildloggerOptions.Export()
	}

	return out
}

// ConvertLogOptions takes a Jasper LogOptions struct and returns an
// equivalent protobuf RPC LogOptions struct. ConvertLogOptions is the
// inverse of (LogOptions) Export().
func ConvertLogOptions(opts jasper.LogOptions) *LogOptions {
	return &LogOptions{
		BufferOptions:      ConvertBufferOptions(opts.BufferOptions),
		BuildloggerOptions: ConvertBuildloggerOptions(opts.BuildloggerOptions),
		DefaultPrefix:      opts.DefaultPrefix,
		FileName:           opts.FileName,
		Format:             ConvertLogFormat(opts.Format),
		InMemoryCap:        int64(opts.InMemoryCap),
		SplunkOptions:      ConvertSplunkOptions(opts.SplunkOptions),
		SumoEndpoint:       opts.SumoEndpoint,
	}
}

// Export takes a protobuf RPC BufferOptions struct and returns the analogous
// Jasper BufferOptions struct.
func (opts *BufferOptions) Export() jasper.BufferOptions {
	return jasper.BufferOptions{
		Buffered: opts.Buffered,
		Duration: time.Duration(opts.Duration),
		MaxSize:  int(opts.MaxSize),
	}
}

// ConvertBufferOptions takes a Jasper BufferOptions struct and returns an
// equivalent protobuf RPC BufferOptions struct. ConvertBufferOptions is the
// inverse of (*BufferOptions) Export().
func ConvertBufferOptions(opts jasper.BufferOptions) *BufferOptions {
	return &BufferOptions{
		Buffered: opts.Buffered,
		Duration: int64(opts.Duration),
		MaxSize:  int64(opts.MaxSize),
	}
}

// Export takes a protobuf RPC BuildloggerOptions struct and returns the
// analogous grip/send.BuildloggerConfig struct.
func (opts BuildloggerOptions) Export() send.BuildloggerConfig {
	return send.BuildloggerConfig{
		CreateTest: opts.CreateTest,
		URL:        opts.Url,
		Number:     int(opts.Number),
		Phase:      opts.Phase,
		Builder:    opts.Builder,
		Test:       opts.Test,
		Command:    opts.Command,
	}
}

// ConvertBuildloggerOptions takes a grip/send.BuildloggerConfig and returns an
// equivalent protobuf RPC BuildloggerOptions struct.
// ConvertBuildloggerOptions is the inverse of (BuildloggerOptions) Export().
func ConvertBuildloggerOptions(opts send.BuildloggerConfig) *BuildloggerOptions {
	return &BuildloggerOptions{
		CreateTest: opts.CreateTest,
		Url:        opts.URL,
		Number:     int64(opts.Number),
		Phase:      opts.Phase,
		Builder:    opts.Builder,
		Test:       opts.Test,
		Command:    opts.Command,
	}
}

// Export takes a protobuf RPC BuildloggerURLs struct and returns the analogous
// []string.
func (u *BuildloggerURLs) Export() []string {
	return append([]string{}, u.Urls...)
}

// ConvertBuildloggerURLs takes a []string and returns the analogous protobuf
// RPC BuildloggerURLs struct. ConvertBuildloggerURLs is the
// inverse of (*BuildloggerURLs) Export().
func ConvertBuildloggerURLs(urls []string) *BuildloggerURLs {
	u := &BuildloggerURLs{Urls: []string{}}
	u.Urls = append(u.Urls, urls...)
	return u
}

// Export takes a protobuf RPC SplunkOptions struct and returns the
// analogous grip/send.SplunkConnectionInfo struct.
func (opts SplunkOptions) Export() send.SplunkConnectionInfo {
	return send.SplunkConnectionInfo{
		ServerURL: opts.Url,
		Token:     opts.Token,
		Channel:   opts.Channel,
	}
}

// ConvertSplunkOptions takes a grip/send.SplunkConnectionInfo and returns the
// analogous protobuf RPC SplunkOptions struct. ConvertSplunkOptions is the
// inverse of (SplunkOptions) Export().
func ConvertSplunkOptions(opts send.SplunkConnectionInfo) *SplunkOptions {
	return &SplunkOptions{
		Url:     opts.ServerURL,
		Token:   opts.Token,
		Channel: opts.Channel,
	}
}

// Export takes a protobuf RPC LogFormat struct and returns the analogous
// Jasper LogFormat struct.
func (f LogFormat) Export() jasper.LogFormat {
	switch f {
	case LogFormat_LOGFORMATDEFAULT:
		return jasper.LogFormatDefault
	case LogFormat_LOGFORMATJSON:
		return jasper.LogFormatJSON
	case LogFormat_LOGFORMATPLAIN:
		return jasper.LogFormatPlain
	default:
		return jasper.LogFormatInvalid
	}
}

// ConvertLogFormat takes a Jasper LogFormat struct and returns an
// equivalent protobuf RPC LogFormat struct. ConvertLogFormat is the
// inverse of (LogFormat) Export().
func ConvertLogFormat(f jasper.LogFormat) LogFormat {
	switch f {
	case jasper.LogFormatDefault:
		return LogFormat_LOGFORMATDEFAULT
	case jasper.LogFormatJSON:
		return LogFormat_LOGFORMATJSON
	case jasper.LogFormatPlain:
		return LogFormat_LOGFORMATPLAIN
	default:
		return LogFormat_LOGFORMATUNKNOWN
	}
}

// Export takes a protobuf RPC BuildOptions struct and returns the analogous
// bond.BuildOptions struct.
func (opts *BuildOptions) Export() bond.BuildOptions {
	return bond.BuildOptions{
		Target:  opts.Target,
		Arch:    bond.MongoDBArch(opts.Arch),
		Edition: bond.MongoDBEdition(opts.Edition),
		Debug:   opts.Debug,
	}
}

// ConvertBuildOptions takes a bond BuildOptions struct and returns an
// equivalent protobuf RPC BuildOptions struct. ConvertBuildOptions is the
// inverse of (*BuildOptions) Export().
func ConvertBuildOptions(opts bond.BuildOptions) *BuildOptions {
	return &BuildOptions{
		Target:  opts.Target,
		Arch:    string(opts.Arch),
		Edition: string(opts.Edition),
		Debug:   opts.Debug,
	}
}

// Export takes a protobuf RPC MongoDBDownloadOptions struct and returns the
// analogous Jasper MongoDBDownloadOptions struct.
func (opts *MongoDBDownloadOptions) Export() jasper.MongoDBDownloadOptions {
	jopts := jasper.MongoDBDownloadOptions{
		BuildOpts: opts.BuildOpts.Export(),
		Path:      opts.Path,
		Releases:  make([]string, 0, len(opts.Releases)),
	}
	jopts.Releases = append(jopts.Releases, opts.Releases...)
	return jopts
}

// ConvertMongoDBDownloadOptions takes a Jasper MongoDBDownloadOptions struct
// and returns an equivalent protobuf RPC MongoDBDownloadOptions struct.
// ConvertMongoDBDownloadOptions is the
// inverse of (*MongoDBDownloadOptions) Export().
func ConvertMongoDBDownloadOptions(jopts jasper.MongoDBDownloadOptions) *MongoDBDownloadOptions {
	opts := &MongoDBDownloadOptions{
		BuildOpts: ConvertBuildOptions(jopts.BuildOpts),
		Path:      jopts.Path,
		Releases:  make([]string, 0, len(jopts.Releases)),
	}
	opts.Releases = append(opts.Releases, jopts.Releases...)
	return opts
}

// Export takes a protobuf RPC CacheOptions struct and returns the analogous
// Jasper CacheOptions struct.
func (opts *CacheOptions) Export() jasper.CacheOptions {
	return jasper.CacheOptions{
		Disabled:   opts.Disabled,
		PruneDelay: time.Duration(opts.PruneDelaySeconds) * time.Second,
		MaxSize:    int(opts.MaxSize),
	}
}

// ConvertCacheOptions takes a Jasper CacheOptions struct and returns an
// equivalent protobuf RPC CacheOptions struct. ConvertCacheOptions is the
// inverse of (*CacheOptions) Export().
func ConvertCacheOptions(jopts jasper.CacheOptions) *CacheOptions {
	return &CacheOptions{
		Disabled:          jopts.Disabled,
		PruneDelaySeconds: int64(jopts.PruneDelay / time.Second),
		MaxSize:           int64(jopts.MaxSize),
	}
}

// Export takes a protobuf RPC DownloadInfo struct and returns the analogous
// Jasper DownloadInfo struct.
func (info *DownloadInfo) Export() jasper.DownloadInfo {
	return jasper.DownloadInfo{
		Path:        info.Path,
		URL:         info.Url,
		ArchiveOpts: info.ArchiveOpts.Export(),
	}
}

// ConvertDownloadInfo takes a Jasper DownloadInfo struct and returns an
// equivalent protobuf RPC DownloadInfo struct. ConvertDownloadInfo is the
// inverse of (*DownloadInfo) Export().
func ConvertDownloadInfo(info jasper.DownloadInfo) *DownloadInfo {
	return &DownloadInfo{
		Path:        info.Path,
		Url:         info.URL,
		ArchiveOpts: ConvertArchiveOptions(info.ArchiveOpts),
	}
}

// Export takes a protobuf RPC ArchiveFormat struct and returns the analogous
// Jasper ArchiveFormat struct.
func (format ArchiveFormat) Export() jasper.ArchiveFormat {
	switch format {
	case ArchiveFormat_ARCHIVEAUTO:
		return jasper.ArchiveAuto
	case ArchiveFormat_ARCHIVETARGZ:
		return jasper.ArchiveTarGz
	case ArchiveFormat_ARCHIVEZIP:
		return jasper.ArchiveZip
	default:
		return jasper.ArchiveFormat("")
	}
}

// ConvertArchiveFormat takes a Jasper ArchiveFormat struct and returns an
// equivalent protobuf RPC ArchiveFormat struct. ConvertArchiveFormat is the
// inverse of (ArchiveFormat) Export().
func ConvertArchiveFormat(format jasper.ArchiveFormat) ArchiveFormat {
	switch format {
	case jasper.ArchiveAuto:
		return ArchiveFormat_ARCHIVEAUTO
	case jasper.ArchiveTarGz:
		return ArchiveFormat_ARCHIVETARGZ
	case jasper.ArchiveZip:
		return ArchiveFormat_ARCHIVEZIP
	default:
		return ArchiveFormat_ARCHIVEUNKNOWN
	}
}

// Export takes a protobuf RPC ArchiveOptions struct and returns the analogous
// Jasper ArchiveOptions struct.
func (opts ArchiveOptions) Export() jasper.ArchiveOptions {
	return jasper.ArchiveOptions{
		ShouldExtract: opts.ShouldExtract,
		Format:        opts.Format.Export(),
		TargetPath:    opts.TargetPath,
	}
}

// ConvertArchiveOptions takes a Jasper ArchiveOptions struct and returns an
// equivalent protobuf RPC ArchiveOptions struct. ConvertArchiveOptions is the
// inverse of (ArchiveOptions) Export().
func ConvertArchiveOptions(opts jasper.ArchiveOptions) *ArchiveOptions {
	return &ArchiveOptions{
		ShouldExtract: opts.ShouldExtract,
		Format:        ConvertArchiveFormat(opts.Format),
		TargetPath:    opts.TargetPath,
	}
}

// Export takes a protobuf RPC SignalTriggerParams struct and returns the analogous
// Jasper process ID and SignalTriggerID.
func (t SignalTriggerParams) Export() (string, jasper.SignalTriggerID) {
	return t.ProcessID.Value, t.SignalTriggerID.Export()
}

// ConvertSignalTriggerParams takes a Jasper process ID and a SignalTriggerID
// and returns an equivalent protobuf RPC SignalTriggerParams struct.
// ConvertSignalTriggerParams is the inverse of (SignalTriggerParams) Export().
func ConvertSignalTriggerParams(jasperProcessID string, signalTriggerID jasper.SignalTriggerID) *SignalTriggerParams {
	return &SignalTriggerParams{
		ProcessID:       &JasperProcessID{Value: jasperProcessID},
		SignalTriggerID: ConvertSignalTriggerID(signalTriggerID),
	}
}

// Export takes a protobuf RPC SignalTriggerID and returns the analogous
// Jasper SignalTriggerID.
func (t SignalTriggerID) Export() jasper.SignalTriggerID {
	switch t {
	case SignalTriggerID_CLEANTERMINATION:
		return jasper.CleanTerminationSignalTrigger
	default:
		return jasper.SignalTriggerID("")
	}
}

// ConvertSignalTriggerID takes a Jasper SignalTriggerID and returns an
// equivalent protobuf RPC SignalTriggerID. ConvertSignalTrigger is the
// inverse of (SignalTriggerID) Export().
func ConvertSignalTriggerID(id jasper.SignalTriggerID) SignalTriggerID {
	switch id {
	case jasper.CleanTerminationSignalTrigger:
		return SignalTriggerID_CLEANTERMINATION
	default:
		return SignalTriggerID_NONE
	}
}

// Export takes a protobuf RPC LogStream and returns the analogous
// Jasper LogStream.
func (l *LogStream) Export() jasper.LogStream {
	return jasper.LogStream{
		Logs: l.Logs,
		Done: l.Done,
	}
}

// ConvertLogStream takes a Jasper LogStream and returns an
// equivalent protobuf RPC LogStream. ConvertLogStream is the
// inverse of (*LogStream) Export().
func ConvertLogStream(l jasper.LogStream) *LogStream {
	return &LogStream{
		Logs: l.Logs,
		Done: l.Done,
	}
}
