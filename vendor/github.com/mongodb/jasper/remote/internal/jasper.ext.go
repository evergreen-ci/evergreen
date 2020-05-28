package internal

import (
	"bytes"
	"os"
	"syscall"
	"time"

	"github.com/golang/protobuf/ptypes"
	duration "github.com/golang/protobuf/ptypes/duration"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
	"github.com/tychoish/bond"
)

// Export takes a protobuf RPC CreateOptions struct and returns the analogous
// Jasper CreateOptions struct. It is not safe to concurrently access the
// exported RPC CreateOptions and the returned Jasper CreateOptions.
func (opts *CreateOptions) Export() *options.Create {
	out := &options.Create{
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
func ConvertCreateOptions(opts *options.Create) *CreateOptions {
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
	case Signals_ABRT:
		return syscall.SIGABRT
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
func ConvertFilter(f options.Filter) *Filter {
	switch f {
	case options.All:
		return &Filter{Name: FilterSpecifications_ALL}
	case options.Running:
		return &Filter{Name: FilterSpecifications_RUNNING}
	case options.Terminated:
		return &Filter{Name: FilterSpecifications_TERMINATED}
	case options.Failed:
		return &Filter{Name: FilterSpecifications_FAILED}
	case options.Successful:
		return &Filter{Name: FilterSpecifications_SUCCESSFUL}
	default:
		return nil
	}
}

// Export takes a protobuf RPC LogType struct and returns the analogous
// Jasper LogType struct.
func (lt LogType) Export() options.LogType {
	switch lt {
	case LogType_LOGBUILDLOGGERV2:
		return options.LogBuildloggerV2
	case LogType_LOGBUILDLOGGERV3:
		return options.LogBuildloggerV3
	case LogType_LOGDEFAULT:
		return options.LogDefault
	case LogType_LOGFILE:
		return options.LogFile
	case LogType_LOGINHERIT:
		return options.LogInherit
	case LogType_LOGSPLUNK:
		return options.LogSplunk
	case LogType_LOGSUMOLOGIC:
		return options.LogSumologic
	case LogType_LOGINMEMORY:
		return options.LogInMemory
	default:
		return options.LogType("")
	}
}

// ConvertLogType takes a Jasper LogType struct and returns an
// equivalent protobuf RPC LogType struct. ConvertLogType is the
// inverse of (LogType) Export().
func ConvertLogType(lt options.LogType) LogType {
	switch lt {
	case options.LogBuildloggerV2:
		return LogType_LOGBUILDLOGGERV2
	case options.LogBuildloggerV3:
		return LogType_LOGBUILDLOGGERV3
	case options.LogDefault:
		return LogType_LOGDEFAULT
	case options.LogFile:
		return LogType_LOGFILE
	case options.LogInherit:
		return LogType_LOGINHERIT
	case options.LogSplunk:
		return LogType_LOGSPLUNK
	case options.LogSumologic:
		return LogType_LOGSUMOLOGIC
	case options.LogInMemory:
		return LogType_LOGINMEMORY
	default:
		return LogType_LOGUNKNOWN
	}
}

// Export takes a protobuf RPC OutputOptions struct and returns the analogous
// Jasper OutputOptions struct.
func (opts OutputOptions) Export() options.Output {
	loggers := []options.Logger{}
	for _, logger := range opts.Loggers {
		loggers = append(loggers, logger.Export())
	}
	return options.Output{
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
func ConvertOutputOptions(opts options.Output) OutputOptions {
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
func (logger Logger) Export() options.Logger {
	return options.Logger{
		Type:    logger.LogType.Export(),
		Options: logger.LogOptions.Export(),
	}
}

// ConvertLogger takes a Jasper Logger struct and returns an
// equivalent protobuf RPC Logger struct. ConvertLogger is the
// inverse of (Logger) Export().
func ConvertLogger(logger options.Logger) *Logger {
	return &Logger{
		LogType:    ConvertLogType(logger.Type),
		LogOptions: ConvertLogOptions(logger.Options),
	}
}

// Export takes a protobuf RPC LogOptions struct and returns the analogous
// Jasper LogOptions struct.
func (opts LogOptions) Export() options.Log {
	out := options.Log{
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
	if opts.Level != nil {
		out.Level = opts.Level.Export()
	}

	return out
}

// ConvertLogOptions takes a Jasper LogOptions struct and returns an
// equivalent protobuf RPC LogOptions struct. ConvertLogOptions is the
// inverse of (LogOptions) Export().
func ConvertLogOptions(opts options.Log) *LogOptions {
	return &LogOptions{
		BufferOptions:      ConvertBufferOptions(opts.BufferOptions),
		BuildloggerOptions: ConvertBuildloggerOptions(opts.BuildloggerOptions),
		DefaultPrefix:      opts.DefaultPrefix,
		FileName:           opts.FileName,
		Format:             ConvertLogFormat(opts.Format),
		InMemoryCap:        int64(opts.InMemoryCap),
		Level:              ConvertLogLevel(opts.Level),
		SplunkOptions:      ConvertSplunkOptions(opts.SplunkOptions),
		SumoEndpoint:       opts.SumoEndpoint,
	}
}

// Export takes a protobuf RPC BufferOptions struct and returns the analogous
// Jasper BufferOptions struct.
func (opts *BufferOptions) Export() options.Buffer {
	return options.Buffer{
		Buffered: opts.Buffered,
		Duration: time.Duration(opts.Duration),
		MaxSize:  int(opts.MaxSize),
	}
}

// ConvertBufferOptions takes a Jasper BufferOptions struct and returns an
// equivalent protobuf RPC BufferOptions struct. ConvertBufferOptions is the
// inverse of (*BufferOptions) Export().
func ConvertBufferOptions(opts options.Buffer) *BufferOptions {
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
func (f LogFormat) Export() options.LogFormat {
	switch f {
	case LogFormat_LOGFORMATDEFAULT:
		return options.LogFormatDefault
	case LogFormat_LOGFORMATJSON:
		return options.LogFormatJSON
	case LogFormat_LOGFORMATPLAIN:
		return options.LogFormatPlain
	default:
		return options.LogFormatInvalid
	}
}

// ConvertLogFormat takes a Jasper LogFormat struct and returns an
// equivalent protobuf RPC LogFormat struct. ConvertLogFormat is the
// inverse of (LogFormat) Export().
func ConvertLogFormat(f options.LogFormat) LogFormat {
	switch f {
	case options.LogFormatDefault:
		return LogFormat_LOGFORMATDEFAULT
	case options.LogFormatJSON:
		return LogFormat_LOGFORMATJSON
	case options.LogFormatPlain:
		return LogFormat_LOGFORMATPLAIN
	default:
		return LogFormat_LOGFORMATUNKNOWN
	}
}

// Export takes a protobuf RPC LogLevel struct and returns the analogous send
// LevelInfo struct.
func (l *LogLevel) Export() send.LevelInfo {
	return send.LevelInfo{Threshold: level.Priority(l.Threshold), Default: level.Priority(l.Default)}
}

// ConvertLogLevel takes a send LevelInfo struct and returns an equivalent
// protobuf RPC LogLevel struct. ConvertLogLevel is the inverse of
// (*LogLevel) Export().
func ConvertLogLevel(l send.LevelInfo) *LogLevel {
	return &LogLevel{Threshold: int32(l.Threshold), Default: int32(l.Default)}
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
func (opts *MongoDBDownloadOptions) Export() options.MongoDBDownload {
	jopts := options.MongoDBDownload{
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
func ConvertMongoDBDownloadOptions(jopts options.MongoDBDownload) *MongoDBDownloadOptions {
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
func (opts *CacheOptions) Export() options.Cache {
	return options.Cache{
		Disabled:   opts.Disabled,
		PruneDelay: time.Duration(opts.PruneDelaySeconds) * time.Second,
		MaxSize:    int(opts.MaxSize),
	}
}

// ConvertCacheOptions takes a Jasper CacheOptions struct and returns an
// equivalent protobuf RPC CacheOptions struct. ConvertCacheOptions is the
// inverse of (*CacheOptions) Export().
func ConvertCacheOptions(jopts options.Cache) *CacheOptions {
	return &CacheOptions{
		Disabled:          jopts.Disabled,
		PruneDelaySeconds: int64(jopts.PruneDelay / time.Second),
		MaxSize:           int64(jopts.MaxSize),
	}
}

// Export takes a protobuf RPC DownloadInfo struct and returns the analogous
// options.Download struct.
func (opts *DownloadInfo) Export() options.Download {
	return options.Download{
		Path:        opts.Path,
		URL:         opts.Url,
		ArchiveOpts: opts.ArchiveOpts.Export(),
	}
}

// ConvertDownloadOptions takes an options.Download struct and returns an
// equivalent protobuf RPC DownloadInfo struct. ConvertDownloadOptions is the
// inverse of (*DownloadInfo) Export().
func ConvertDownloadOptions(opts options.Download) *DownloadInfo {
	return &DownloadInfo{
		Path:        opts.Path,
		Url:         opts.URL,
		ArchiveOpts: ConvertArchiveOptions(opts.ArchiveOpts),
	}
}

// Export takes a protobuf RPC WriteFileInfo struct and returns the analogous
// options.WriteFile struct.
func (opts *WriteFileInfo) Export() options.WriteFile {
	return options.WriteFile{
		Path:    opts.Path,
		Content: opts.Content,
		Append:  opts.Append,
		Perm:    os.FileMode(opts.Perm),
	}
}

// ConvertWriteFileOptions takes an options.WriteFile struct and returns an
// equivalent protobuf RPC WriteFileInfo struct. ConvertWriteFileOptions is the
// inverse of (*WriteFileInfo) Export().
func ConvertWriteFileOptions(opts options.WriteFile) *WriteFileInfo {
	return &WriteFileInfo{
		Path:    opts.Path,
		Content: opts.Content,
		Append:  opts.Append,
		Perm:    uint32(opts.Perm),
	}
}

// Export takes a protobuf RPC ArchiveFormat struct and returns the analogous
// Jasper ArchiveFormat struct.
func (format ArchiveFormat) Export() options.ArchiveFormat {
	switch format {
	case ArchiveFormat_ARCHIVEAUTO:
		return options.ArchiveAuto
	case ArchiveFormat_ARCHIVETARGZ:
		return options.ArchiveTarGz
	case ArchiveFormat_ARCHIVEZIP:
		return options.ArchiveZip
	default:
		return options.ArchiveFormat("")
	}
}

// ConvertArchiveFormat takes a Jasper ArchiveFormat struct and returns an
// equivalent protobuf RPC ArchiveFormat struct. ConvertArchiveFormat is the
// inverse of (ArchiveFormat) Export().
func ConvertArchiveFormat(format options.ArchiveFormat) ArchiveFormat {
	switch format {
	case options.ArchiveAuto:
		return ArchiveFormat_ARCHIVEAUTO
	case options.ArchiveTarGz:
		return ArchiveFormat_ARCHIVETARGZ
	case options.ArchiveZip:
		return ArchiveFormat_ARCHIVEZIP
	default:
		return ArchiveFormat_ARCHIVEUNKNOWN
	}
}

// Export takes a protobuf RPC ArchiveOptions struct and returns the analogous
// Jasper ArchiveOptions struct.
func (opts ArchiveOptions) Export() options.Archive {
	return options.Archive{
		ShouldExtract: opts.ShouldExtract,
		Format:        opts.Format.Export(),
		TargetPath:    opts.TargetPath,
	}
}

// ConvertArchiveOptions takes a Jasper ArchiveOptions struct and returns an
// equivalent protobuf RPC ArchiveOptions struct. ConvertArchiveOptions is the
// inverse of (ArchiveOptions) Export().
func ConvertArchiveOptions(opts options.Archive) *ArchiveOptions {
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

// Export converts the rpc type to jasper's equivalent option type.
func (o *ScriptingOptions) Export() (options.ScriptingHarness, error) {
	switch val := o.Value.(type) {
	case *ScriptingOptions_Golang:
		return &options.ScriptingGolang{
			Gopath:         val.Golang.Gopath,
			Goroot:         val.Golang.Goroot,
			Packages:       val.Golang.Packages,
			Context:        val.Golang.Context,
			WithUpdate:     val.Golang.WithUpdate,
			CachedDuration: time.Duration(o.Duration),
			Environment:    o.Environment,
			Output:         o.Output.Export(),
		}, nil
	case *ScriptingOptions_Python:
		return &options.ScriptingPython{
			VirtualEnvPath:        val.Python.VirtualEnvPath,
			RequirementsFilePath:  val.Python.RequirementsPath,
			HostPythonInterpreter: val.Python.HostPython,
			Packages:              val.Python.Packages,
			LegacyPython:          val.Python.LegacyPython,
			AddTestRequirements:   val.Python.AddTestDeps,
			CachedDuration:        time.Duration(o.Duration),
			Environment:           o.Environment,
			Output:                o.Output.Export(),
		}, nil
	case *ScriptingOptions_Roswell:
		return &options.ScriptingRoswell{
			Path:           val.Roswell.Path,
			Systems:        val.Roswell.Systems,
			Lisp:           val.Roswell.Lisp,
			CachedDuration: time.Duration(o.Duration),
			Environment:    o.Environment,
			Output:         o.Output.Export(),
		}, nil
	default:
		return nil, errors.Errorf("invalid scripting options type %T", val)
	}
}

func ConvertScriptingOptions(opts options.ScriptingHarness) *ScriptingOptions {
	switch val := opts.(type) {
	case *options.ScriptingGolang:
		out := ConvertOutputOptions(val.Output)
		return &ScriptingOptions{
			Duration:    int64(val.CachedDuration),
			Environment: val.Environment,
			Output:      &out,
			Value: &ScriptingOptions_Golang{
				Golang: &ScriptingOptionsGolang{
					Gopath:     val.Gopath,
					Goroot:     val.Goroot,
					Packages:   val.Packages,
					Context:    val.Context,
					WithUpdate: val.WithUpdate,
				},
			},
		}
	case *options.ScriptingPython:
		out := ConvertOutputOptions(val.Output)
		return &ScriptingOptions{
			Duration:    int64(val.CachedDuration),
			Environment: val.Environment,
			Output:      &out,
			Value: &ScriptingOptions_Python{
				Python: &ScriptingOptionsPython{
					VirtualEnvPath:   val.VirtualEnvPath,
					RequirementsPath: val.RequirementsFilePath,
					HostPython:       val.HostPythonInterpreter,
					Packages:         val.Packages,
					LegacyPython:     val.LegacyPython,
					AddTestDeps:      val.AddTestRequirements,
				},
			},
		}
	case *options.ScriptingRoswell:
		out := ConvertOutputOptions(val.Output)
		return &ScriptingOptions{
			Duration:    int64(val.CachedDuration),
			Environment: val.Environment,
			Output:      &out,
			Value: &ScriptingOptions_Roswell{
				Roswell: &ScriptingOptionsRoswell{
					Path:    val.Path,
					Systems: val.Systems,
					Lisp:    val.Lisp,
				},
			},
		}
	default:
		grip.Criticalf("'%T' is not supported", opts)
		return nil
	}
}

func mustConvertTimestamp(t time.Time) *timestamp.Timestamp {
	out, err := ptypes.TimestampProto(t)
	if err != nil {
		panic(err)
	}
	return out
}

func mustConvertPTimestamp(t *timestamp.Timestamp) time.Time {
	out, err := ptypes.Timestamp(t)
	if err != nil {
		panic(err)
	}
	return out
}

func ConvertScriptingTestResults(res []scripting.TestResult) []*ScriptingHarnessTestResult {
	out := make([]*ScriptingHarnessTestResult, len(res))
	for idx, r := range res {
		out[idx] = &ScriptingHarnessTestResult{
			Name:     r.Name,
			StartAt:  mustConvertTimestamp(r.StartAt),
			Duration: ptypes.DurationProto(r.Duration),
			Outcome:  string(r.Outcome),
		}
	}
	return out
}

func (r *ScriptingHarnessTestResponse) Export() []scripting.TestResult {
	out := make([]scripting.TestResult, len(r.Results))
	for idx, res := range r.Results {
		out[idx] = scripting.TestResult{
			Name:     res.Name,
			StartAt:  mustConvertPTimestamp(res.StartAt),
			Duration: mustConvertDuration(res.Duration),
			Outcome:  scripting.TestOutcome(res.Outcome),
		}
	}
	return out
}

func mustConvertDuration(in *duration.Duration) time.Duration {
	dur, err := ptypes.Duration(in)
	if err != nil {
		panic(err)
	}

	return dur
}

func (a *ScriptingHarnessTestArgs) Export() []scripting.TestOptions {
	out := make([]scripting.TestOptions, len(a.Options))
	for idx, opts := range a.Options {
		out[idx] = scripting.TestOptions{
			Name:    opts.Name,
			Args:    opts.Args,
			Pattern: opts.Pattern,
			Timeout: mustConvertDuration(opts.Timeout),
			Count:   int(opts.Count),
		}
	}
	return out
}

func ConvertScriptingTestOptions(args []scripting.TestOptions) []*ScriptingHarnessTestOptions {
	out := make([]*ScriptingHarnessTestOptions, len(args))
	for idx, opt := range args {
		out[idx] = &ScriptingHarnessTestOptions{
			Name:    opt.Name,
			Args:    opt.Args,
			Pattern: opt.Pattern,
			Timeout: ptypes.DurationProto(opt.Timeout),
			Count:   int32(opt.Count),
		}
	}
	return out
}
