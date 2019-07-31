package cli

import (
	"encoding/json"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/jasper"
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
func ExtractOutcomeResponse(input []byte) (OutcomeResponse, error) {
	resp := OutcomeResponse{}
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
func ExtractInfoResponse(input []byte) (InfoResponse, error) {
	resp := InfoResponse{}
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
func ExtractInfosResponse(input []byte) (InfosResponse, error) {
	resp := InfosResponse{}
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
func ExtractTagsResponse(input []byte) (TagsResponse, error) {
	resp := TagsResponse{}
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
func ExtractRunningResponse(input []byte) (RunningResponse, error) {
	resp := RunningResponse{}
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
func ExtractCompleteResponse(input []byte) (CompleteResponse, error) {
	resp := CompleteResponse{}
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
func ExtractWaitResponse(input []byte) (WaitResponse, error) {
	resp := WaitResponse{}
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
func ExtractServiceStatusResponse(input []byte) (ServiceStatusResponse, error) {
	resp := ServiceStatusResponse{}
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
func ExtractLogStreamResponse(input []byte) (LogStreamResponse, error) {
	resp := LogStreamResponse{}
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
func ExtractBuildloggerURLsResponse(input []byte) (BuildloggerURLsResponse, error) {
	resp := BuildloggerURLsResponse{}
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

type IDResponse struct {
	OutcomeResponse `json:"outcome"`
	ID              string `json:"id,omitempty"`
}

func ExtractIDResponse(input []byte) (IDResponse, error) {
	resp := IDResponse{}
	if err := json.Unmarshal(input, &resp); err != nil {
		return resp, errors.Wrap(err, unmarshalFailed)
	}
	return resp, resp.successOrError()
}

// IDInput represents CLI-specific input representing a Jasper process ID.
type IDInput struct {
	ID string `json:"id"`
}

// Validate checks that the Jasper process ID is non-empty.
func (in *IDInput) Validate() error {
	if len(in.ID) == 0 {
		return errors.New("Jasper process ID must not be empty")
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

// CommandInput represents CLI-specific input to create a jasper.Command.
type CommandInput struct {
	Commands        [][]string           `json:"commands"`
	Background      bool                 `json:"background,omitempty"`
	CreateOptions   jasper.CreateOptions `json:"create_options,omitempty"`
	Priority        level.Priority       `json:"priority,omitempty"`
	ContinueOnError bool                 `json:"continue_on_error,omitempty"`
	IgnoreError     bool                 `json:"ignore_error,omitempty"`
}

// Validate checks that the input to the jasper.Command is valid.
func (in *CommandInput) Validate() error {
	catcher := grip.NewBasicCatcher()
	// The semantics of CreateOptions expects Args to be non-empty, but
	// jasper.Command ignores (CreateOptions).Args.
	if len(in.CreateOptions.Args) == 0 {
		in.CreateOptions.Args = []string{""}
	}
	catcher.Add(in.CreateOptions.Validate())
	if in.Priority != 0 && !level.IsValidPriority(in.Priority) {
		catcher.Add(errors.New("priority is not in the valid range of values"))
	}
	if len(in.Commands) == 0 {
		catcher.Add(errors.New("must specify at least one command"))
	}
	return catcher.Resolve()
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
	Filter jasper.Filter
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
