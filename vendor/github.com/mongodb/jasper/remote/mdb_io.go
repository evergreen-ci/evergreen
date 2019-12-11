package remote

import (
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
)

// infoRequest represents a request for runtime information regarding the
// process given by ID.
type infoRequest struct {
	ID string `bson:"info"`
}

// infoResponse represents a response indicating runtime information for a
// process.
type infoResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	Info                jasper.ProcessInfo `bson:"info"`
}

func makeInfoResponse(info jasper.ProcessInfo) infoResponse {
	return infoResponse{Info: info, ErrorResponse: shell.MakeSuccessResponse()}
}

// runningRequest represents a request for the running state of the process
// given by ID.
type runningRequest struct {
	ID string `bson:"running"`
}

// runningResponse represents a response indicating the running state of a
// process.
type runningResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	Running             bool `bson:"running"`
}

func makeRunningResponse(running bool) runningResponse {
	return runningResponse{Running: running, ErrorResponse: shell.MakeSuccessResponse()}
}

// completeRequest represents a request for the completion status of the process
// given by ID.
type completeRequest struct {
	ID string `bson:"complete"`
}

// completeResponse represents a response indicating the completion status of a
// process.
type completeResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	Complete            bool `bson:"complete"`
}

func makeCompleteResponse(complete bool) completeResponse {
	return completeResponse{Complete: complete, ErrorResponse: shell.MakeSuccessResponse()}
}

// waitRequest represents a request for the wait status of the process given  by
// ID.
type waitRequest struct {
	ID string `bson:"wait"`
}

// waitResponse represents a response indicating the exit code and error of
// a waited process.
type waitResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	ExitCode            int `bson:"exit_code"`
}

func makeWaitResponse(exitCode int, err error) waitResponse {
	return waitResponse{ExitCode: exitCode, ErrorResponse: shell.MakeErrorResponse(true, err)}
}

// respawnRequest represents a request to respawn the process given by ID.
type respawnRequest struct {
	ID string `bson:"respawn"`
}

// signalRequest represents a request to send a signal to the process given by
// ID.
type signalRequest struct {
	Params struct {
		ID     string `bson:"id"`
		Signal int    `bson:"signal"`
	} `bson:"signal"`
}

// registerSignalTriggerIDRequest represents a request to register the signal
// trigger ID on the process given by ID.
type registerSignalTriggerIDRequest struct {
	Params struct {
		ID              string                 `bson:"id"`
		SignalTriggerID jasper.SignalTriggerID `bson:"signal_trigger_id"`
	} `bson:"register_signal_trigger_id"`
}

// tagRequest represents a request to associate the process given by ID with the
// tag.
type tagRequest struct {
	Params struct {
		ID  string `bson:"id"`
		Tag string `bson:"tag"`
	} `bson:"add_tag"`
}

// getTagsRequest represents a request to get all the tags for the process given
// by ID.
type getTagsRequest struct {
	ID string `bson:"get_tags"`
}

// getTagsResponse represents a response indicating the tags of a process.
type getTagsResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	Tags                []string `bson:"tags"`
}

func makeGetTagsResponse(tags []string) getTagsResponse {
	return getTagsResponse{Tags: tags, ErrorResponse: shell.MakeSuccessResponse()}
}

// resetTagsRequest represents a request to clear all the tags for the process
// given by ID.
type resetTagsRequest struct {
	ID string `bson:"reset_tags"`
}

// idRequest represents a request to get the ID associated with the service
// manager.
type idRequest struct {
	ID int `bson:"id"`
}

// idResponse requests a response indicating the service manager's ID.
type idResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	ID                  string `bson:"id"`
}

func makeIDResponse(id string) idResponse {
	return idResponse{ID: id, ErrorResponse: shell.MakeSuccessResponse()}
}

// createProcessRequest represents a request to create a process with the given
// options.
type createProcessRequest struct {
	Options options.Create `bson:"create_process"`
}

// listRequest represents a request to get information regarding the processes
// matching the given filter.
type listRequest struct {
	Filter options.Filter `bson:"list"`
}

// groupRequest represents a request to get information regarding the processes
// matching the given tag.
type groupRequest struct {
	Tag string `bson:"group"`
}

// getProcessRequest represents a request to get information regarding the
// process given by ID.
type getProcessRequest struct {
	ID string `bson:"get_process"`
}

// infosResponse represents a response indicating the runtime information for
// multiple processes.
type infosResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	Infos               []jasper.ProcessInfo `bson:"infos"`
}

func makeInfosResponse(infos []jasper.ProcessInfo) infosResponse {
	return infosResponse{Infos: infos, ErrorResponse: shell.MakeSuccessResponse()}
}

// clearRequest represents a request to clear the current processes that have
// completed.
type clearRequest struct {
	Clear int `bson:"clear"`
}

// closeRequest represents a request to terminate all processes.
type closeRequest struct {
	Close int `bson:"close"`
}

type writeFileRequest struct {
	Options options.WriteFile `bson:"write_file"`
}

type configureCacheRequest struct {
	Options options.Cache `bson:"configure_cache"`
}

type downloadFileRequest struct {
	Options options.Download `bson:"download_file"`
}

type downloadMongoDBRequest struct {
	Options options.MongoDBDownload `bson:"download_mongodb"`
}

type getLogStreamRequest struct {
	Params struct {
		ID    string `bson:"id"`
		Count int    `bson:"count"`
	} `bson:"get_log_stream"`
}

type getLogStreamResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	LogStream           jasper.LogStream `bson:"log_stream"`
}

func makeGetLogStreamResponse(logs []string, done bool) getLogStreamResponse {
	return getLogStreamResponse{
		LogStream:     jasper.LogStream{Logs: logs, Done: done},
		ErrorResponse: shell.MakeSuccessResponse(),
	}
}

type getBuildloggerURLsRequest struct {
	ID string `bson:"get_buildlogger_urls"`
}

type getBuildloggerURLsResponse struct {
	shell.ErrorResponse `bson:"error_response,inline"`
	URLs                []string `bson:"urls,omitempty"`
}

type signalEventRequest struct {
	Name string `bson:"signal_event"`
}
