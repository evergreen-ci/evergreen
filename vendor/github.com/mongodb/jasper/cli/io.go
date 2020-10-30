package cli

import (
	"encoding/json"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

const (
	unmarshalFailed           = "failed to unmarshal response"
	unspecifiedRequestFailure = "request failed for unspecified reason"
)

// Validator represents an input that can be validated.
type Validator interface {
	Validate() error
}

// OutcomeResponse represents CLI-specific output describing if the request was
// processed successfully and if not, the associated error message.  For other
// responses that compose OutcomeResponse, their results are valid only if
// Success is true.
type OutcomeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// Successful returns whether the request was successfully processed.
func (resp OutcomeResponse) Successful() bool {
	return resp.Success
}

// ErrorMessage returns the error message if the request was not successfully
// processed.
func (resp OutcomeResponse) ErrorMessage() string {
	reason := resp.Message
	if !resp.Successful() && reason == "" {
		reason = unspecifiedRequestFailure
	}
	return reason
}

// ExtractOutcomeResponse unmarshals the input bytes into an OutcomeResponse and
// checks if the request was successful.
func ExtractOutcomeResponse(input json.RawMessage) (OutcomeResponse, error) {
	var resp OutcomeResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

func (resp OutcomeResponse) successOrError() error {
	if !resp.Successful() {
		return errors.New(resp.ErrorMessage())
	}
	return nil
}

func makeOutcomeResponse(err error) *OutcomeResponse {
	if err != nil {
		return &OutcomeResponse{Success: false, Message: err.Error()}
	}
	return &OutcomeResponse{Success: true}
}

// InfoResponse represents represents CLI-specific output containing the request
// outcome and process information.
type InfoResponse struct {
	OutcomeResponse `json:"outcome"`
	Info            jasper.ProcessInfo `json:"info,omitempty"`
}

// ExtractInfoResponse unmarshals the input bytes into an InfoResponse and
// checks if the request was successful.
func ExtractInfoResponse(input json.RawMessage) (InfoResponse, error) {
	var resp InfoResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// InfosResponse represents CLI-specific output containing the request outcome
// and information for multiple processes.
type InfosResponse struct {
	OutcomeResponse `json:"outcome"`
	Infos           []jasper.ProcessInfo `json:"infos,omitempty"`
}

// ExtractInfosResponse unmarshals the input bytes into a TagsResponse and
// checks if the request was successful.
func ExtractInfosResponse(input json.RawMessage) (InfosResponse, error) {
	var resp InfosResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// TagsResponse represents CLI-specific output containing the request outcome
// and tags.
type TagsResponse struct {
	OutcomeResponse `json:"outcome"`
	Tags            []string `json:"tags,omitempty"`
}

// ExtractTagsResponse unmarshals the input bytes into a TagsResponse and checks
// if the request was successful.
func ExtractTagsResponse(input json.RawMessage) (TagsResponse, error) {
	var resp TagsResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// RunningResponse represents CLI-specific output containing the request outcome
// and whether the process is running or not.
type RunningResponse struct {
	OutcomeResponse `json:"outcome"`
	Running         bool `json:"running,omitempty"`
}

// ExtractRunningResponse unmarshals the input bytes into a RunningResponse and
// checks if the request was successful.
func ExtractRunningResponse(input json.RawMessage) (RunningResponse, error) {
	var resp RunningResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// CompleteResponse represents CLI-specific output containing the request
// outcome and whether the process is complete or not.
type CompleteResponse struct {
	OutcomeResponse `json:"outcome"`
	Complete        bool `json:"complete,omitempty"`
}

// ExtractCompleteResponse unmarshals the input bytes into a CompleteResponse and
// checks if the request was successful.
func ExtractCompleteResponse(input json.RawMessage) (CompleteResponse, error) {
	var resp CompleteResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// WaitResponse represents CLI-specific output containing the request outcome,
// the wait exit code, and the error from wait.
type WaitResponse struct {
	OutcomeResponse `json:"outcome"`
	ExitCode        int    `json:"exit_code,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ExtractWaitResponse unmarshals the input bytes into a WaitResponse and checks if the
// request was successful.
func ExtractWaitResponse(input json.RawMessage) (WaitResponse, error) {
	var resp WaitResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	if err := resp.successOrError(); err != nil {
		resp.ExitCode = -1
		return resp, err
	}
	return resp, nil
}

// ServiceStatusResponse represents CLI-specific output containing the request
// outcome and the service status.
type ServiceStatusResponse struct {
	OutcomeResponse `json:"outcome"`
	Status          ServiceStatus `json:"status,omitempty"`
}

// ExtractServiceStatusResponse unmarshals the input bytes into a
// ServiceStatusResponse and checks if the request was successful.
func ExtractServiceStatusResponse(input json.RawMessage) (ServiceStatusResponse, error) {
	var resp ServiceStatusResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// LogStreamResponse represents CLI-specific output containing the log stream
// data.
type LogStreamResponse struct {
	OutcomeResponse  `json:"outcome"`
	jasper.LogStream `json:"log_stream,omitempty"`
}

// ExtractLogStreamResponse unmarshals the input bytes into a LogStreamResponse
// and checks if the request was successful.
func ExtractLogStreamResponse(input json.RawMessage) (LogStreamResponse, error) {
	var resp LogStreamResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// BuildloggerURLsResponse represents CLI-specific output containing the
// Buildlogger URLs for a process.
type BuildloggerURLsResponse struct {
	OutcomeResponse `json:"outcome"`
	URLs            []string `json:"urls,omitempty"`
}

// ExtractBuildloggerURLsResponse unmarshals the input bytes into a
// BuildloggerURLsResponse and checks if the request was successful.
func ExtractBuildloggerURLsResponse(input json.RawMessage) (BuildloggerURLsResponse, error) {
	var resp BuildloggerURLsResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// IDResponse represents represents CLI-specific output containing the ID of the
// resources requested (e.g. a Jasper process ID).
type IDResponse struct {
	OutcomeResponse `json:"outcome"`
	ID              string `json:"id,omitempty"`
}

// ExtractIDResponse unmarshals the input bytes into an IDResponse and checks if
// the request was successful.
func ExtractIDResponse(input json.RawMessage) (IDResponse, error) {
	var resp IDResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// IDInput represents CLI-specific input representing a single ID (e.g. a Jasper
// process ID).
type IDInput struct {
	ID string `json:"id"`
}

// Validate checks that the ID is non-empty.
func (in *IDInput) Validate() error {
	if len(in.ID) == 0 {
		return errors.New("ID must not be empty")
	}
	return nil
}

// SignalInput represents CLI-specific input to signal a Jasper process.
type SignalInput struct {
	ID     string `json:"id"`
	Signal int    `json:"signal"`
}

// Validate checks that the SignalInput has a non-empty Jasper process ID and
// positive Signal.
func (in *SignalInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	if len(in.ID) == 0 {
		catcher.Add(errors.New("Jasper process ID must not be empty"))
	}
	if in.Signal <= 0 {
		catcher.Add(errors.New("signal must be greater than 0"))
	}
	return catcher.Resolve()
}

// SignalTriggerIDInput represents CLI-specific input to attach a signal trigger
// to a Jasper process.
type SignalTriggerIDInput struct {
	ID              string                 `json:"id"`
	SignalTriggerID jasper.SignalTriggerID `json:"signal_trigger_id"`
}

// Validate checks that the SignalTriggerIDInput has a non-empty Jasper process
// ID and a recognized signal trigger ID.
func (in *SignalTriggerIDInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	if len(in.ID) == 0 {
		catcher.Add(errors.New("Jasper process ID must not be empty"))
	}
	_, ok := jasper.GetSignalTriggerFactory(in.SignalTriggerID)
	if !ok {
		return errors.Errorf("could not find signal trigger with id '%s'", in.SignalTriggerID)
	}
	return nil
}

// TagIDInput represents the CLI-specific input for a process with a given tag.
type TagIDInput struct {
	ID  string `json:"id"`
	Tag string `json:"tag"`
}

// Validate checks that the TagIDInput has a non-empty Jasper process ID and a
// non-empty tag.
func (in *TagIDInput) Validate() error {
	if len(in.ID) == 0 {
		return errors.New("Jasper process ID must not be empty")
	}
	if len(in.Tag) == 0 {
		return errors.New("tag must not be empty")
	}
	return nil
}

// TagInput represents the CLI-specific input for process tags.
type TagInput struct {
	Tag string `json:"tag"`
}

// Validate checks that the tag is non-empty.
func (in *TagInput) Validate() error {
	if len(in.Tag) == 0 {
		return errors.New("tag must not be empty")
	}
	return nil
}

// FilterInput represents the CLI-specific input to filter processes.
type FilterInput struct {
	Filter options.Filter
}

// Validate checks that the jasper.Filter is a recognized filter.
func (in *FilterInput) Validate() error {
	return in.Filter.Validate()
}

// LogStreamInput represents the CLI-specific input to stream in-memory logs.
type LogStreamInput struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// Validate checks that the number of logs requested is positive.
func (in *LogStreamInput) Validate() error {
	if in.Count <= 0 {
		return errors.New("count must be greater than zero")
	}
	return nil
}

// EventInput represents the CLI-specific input to signal a named event.
type EventInput struct {
	Name string `json:"name"`
}

// Validate checks that the event name is set.
func (e *EventInput) Validate() error {
	if e.Name == "" {
		return errors.New("event name cannot be empty")
	}
	return nil
}

// ScriptingCreateInput represents CLI-specific input to create a scripting
// harness. The Type signifies the kind of harness that will be created and the
// Payload is the JSON-serialized options to construct the harness.
type ScriptingCreateInput struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Validate checks that a scripting type and payload to populate the harness
// have been given.
func (in *ScriptingCreateInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.Type == "", "scripting type must be defined")
	catcher.NewWhen(in.Payload == nil, "scripting payload must be given")

	return catcher.Resolve()
}

// BuildScriptingCreateInput constructs a ScriptingCreateInput value.
func BuildScriptingCreateInput(in options.ScriptingHarness) (*ScriptingCreateInput, error) {
	out := &ScriptingCreateInput{}
	out.Type = in.Type()

	var err error
	out.Payload, err = json.Marshal(in)
	if err != nil {
		return nil, errors.Wrap(err, "problem building message payload")
	}

	return out, nil
}

// Export builds a native scripting harness container.
func (in *ScriptingCreateInput) Export() (options.ScriptingHarness, error) {
	harness, err := options.NewScriptingHarness(in.Type)
	if err != nil {
		return nil, errors.Wrap(err, "creating scripting harness")
	}

	err = json.Unmarshal(in.Payload, harness)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling scripting harness payload")
	}

	return harness, nil
}

// ScriptingRunInput represents CLI-specific input to run a scripting harness.
type ScriptingRunInput struct {
	ID   string   `json:"id"`
	Args []string `json:"args"`
}

// Validate checks that a scripting harness ID is given.
func (in *ScriptingRunInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.ID == "", "must specify scripting harness ID")
	return catcher.Resolve()
}

// ScriptingRunScriptInput represents CLI-specific input to run a script in
// a scripting harness.
type ScriptingRunScriptInput struct {
	ID     string `json:"id"`
	Script string `json:"script"`
}

// Validate checks that a scripting harness ID and the script to run have been
// given.
func (in *ScriptingRunScriptInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.ID == "", "must specify scripting harness ID")
	catcher.NewWhen(in.Script == "", "must specify script to run")
	return catcher.Resolve()
}

// ScriptingBuildInput represents CLI-specific input to run a build in a
// scripting harness.
type ScriptingBuildInput struct {
	ID        string   `json:"id"`
	Directory string   `json:"directory"`
	Args      []string `json:"args"`
}

// Validate checks that a scripting harness ID has been given.
func (in *ScriptingBuildInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.ID == "", "must specify scripting harness ID")
	return catcher.Resolve()
}

// ScriptingBuildResponse represents CLI-specific output describing the output
// path, if applicable.
type ScriptingBuildResponse struct {
	OutcomeResponse `json:"outcome"`
	Path            string `json:"path"`
}

// ExtractScriptingBuildResponse unmarshals the input bytes into a
// ScriptingBuildResponse and checks if the request was successful.
func ExtractScriptingBuildResponse(input json.RawMessage) (ScriptingBuildResponse, error) {
	var resp ScriptingBuildResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// ScriptingTestInput represents CLI-specific input to run tests in a scripting
// harness.
type ScriptingTestInput struct {
	ID        string                  `json:"id"`
	Directory string                  `json:"directory"`
	Options   []scripting.TestOptions `json:"options"`
}

// Validate checks that a scripting harness ID has been given.
func (in *ScriptingTestInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.ID == "", "must specify scripting harness ID")
	return catcher.Resolve()
}

// ScriptingTestResponse represents CLI-specific output describing the results
// of a test run.
type ScriptingTestResponse struct {
	OutcomeResponse `json:"outcome"`
	Results         []scripting.TestResult `json:"results"`
}

// ExtractScriptingTestResponse unmarshals the input bytes into a
// ScriptingTestResponse and checks if the request was successful.
func ExtractScriptingTestResponse(input json.RawMessage) (ScriptingTestResponse, error) {
	var resp ScriptingTestResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// LoggingCacheCreateInput represents CLI-specific input to create a cached
// logger.
type LoggingCacheCreateInput struct {
	ID     string         `json:"id"`
	Output options.Output `json:"options"`
}

// Validate checks that a cached logger ID has been given and the logger options
// are valid.
func (in *LoggingCacheCreateInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(in.ID == "", "ID must not be empty")
	catcher.Wrap(in.Output.Validate(), "invalid output options")
	return catcher.Resolve()
}

// CachedLoggerResponse represents CLI-specific output describing the logger
// that has been cached in the remote service.
type CachedLoggerResponse struct {
	OutcomeResponse `json:"outcome"`
	Logger          options.CachedLogger `json:"logger"`
}

// ExtractCachedLoggerResponse unmarshals the input bytes into a
// CachedLoggerResponse and checks if the request was successful.
func ExtractCachedLoggerResponse(input json.RawMessage) (CachedLoggerResponse, error) {
	var resp CachedLoggerResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// LoggingCachePruneInput represents CLI-specific input to prune the loggers
// that were accessed before the given time.
type LoggingCachePruneInput struct {
	LastAccessed time.Time `json:"last_accessed"`
}

// Validate is a no-op.
func (in *LoggingCachePruneInput) Validate() error { return nil }

// LoggingCacheLenResponse represents CLI-specific output describing the
// number of cached loggers.
type LoggingCacheLenResponse struct {
	OutcomeResponse `json:"outcome"`
	Length          int `json:"length"`
}

// ExtractLoggingCacheLenResponse unmarshals the input bytes into a
// LoggingCacheLenResponse and checks if the request was successful.
func ExtractLoggingCacheLenResponse(input json.RawMessage) (LoggingCacheLenResponse, error) {
	var resp LoggingCacheLenResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}
