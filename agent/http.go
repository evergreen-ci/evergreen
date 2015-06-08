package agent

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"io/ioutil"
	"net/http"
	"time"
)

var HTTPConflictError = errors.New("Conflict")

// HTTPCommunicator handles communication with the API server. An HTTPCommunicator
// is scoped to a single task, and all communication performed by it is
// only relevant to that running task.
type HTTPCommunicator struct {
	ServerURLRoot string
	TaskId        string
	TaskSecret    string
	MaxAttempts   int
	RetrySleep    time.Duration
	SignalChan    chan Signal
	Logger        *slogger.Logger
	HttpsCert     string
	httpClient    *http.Client
}

// NewHTTPCommunicator returns an initialized HTTPCommunicator.
// The cert parameter may be blank if default system certificates are being used.
func NewHTTPCommunicator(serverURL, taskId, taskSecret, cert string, sigChan chan Signal) (*HTTPCommunicator, error) {
	agentCommunicator := &HTTPCommunicator{
		ServerURLRoot: fmt.Sprintf("%v/api/%v", serverURL, APIVersion),
		TaskId:        taskId,
		TaskSecret:    taskSecret,
		MaxAttempts:   10,
		RetrySleep:    time.Second * 3,
		HttpsCert:     cert,
		SignalChan:    sigChan,
	}

	if agentCommunicator.HttpsCert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(agentCommunicator.HttpsCert)) {
			return nil, errors.New("failed to append HttpsCert to new cert pool")
		}
		tc := &tls.Config{RootCAs: pool}
		tr := &http.Transport{TLSClientConfig: tc}
		agentCommunicator.httpClient = &http.Client{Transport: tr}
	} else {
		agentCommunicator.httpClient = &http.Client{}
	}
	return agentCommunicator, nil
}

// Heartbeat encapsulates heartbeat behavior (i.e., pinging the API server at regular
// intervals to ensure that communication hasn't broken down).
type Heartbeat interface {
	Heartbeat() (bool, error)
}

// TaskJSONCommunicator handles plugin-specific JSON-encoded
// communication with the API server.
type TaskJSONCommunicator struct {
	PluginName string
	TaskCommunicator
}

// TaskPostJSON does an HTTP POST for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskPostJSON(endpoint string, data interface{}) (*http.Response, error) {
	return t.tryPostJSON(fmt.Sprintf("%s/%s", t.PluginName, endpoint), data)
}

// TaskGetJSON does an HTTP GET for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskGetJSON(endpoint string) (*http.Response, error) {
	return t.tryGet(fmt.Sprintf("%s/%s", t.PluginName, endpoint))
}

// TaskPostResults posts a set of test results for the communicator's task.
func (t *TaskJSONCommunicator) TaskPostResults(results *model.TestResults) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.tryPostJSON("results", results)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				err := fmt.Errorf("error posting results: %v", err)
				return util.RetriableError{err}
			}
			body, _ := ioutil.ReadAll(resp.Body)
			bodyErr := fmt.Errorf("error posting results (%v): %v",
				resp.StatusCode, string(body))
			switch resp.StatusCode {
			case http.StatusOK:
				return nil
			case http.StatusBadRequest:
				return bodyErr
			default:
				return util.RetriableError{bodyErr}
			}
		},
	)
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, 10, 1)
	if retryFail {
		return fmt.Errorf("attaching test results failed after %v tries: %v", 10, err)
	}
	return nil
}

// PostTaskFiles is used by the PluginCommunicator interface for attaching task files.
func (t *TaskJSONCommunicator) PostTaskFiles(task_files []*artifact.File) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.tryPostJSON("files", task_files)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				err := fmt.Errorf("error posting results: %v", err)
				return util.RetriableError{err}
			}
			body, readAllErr := ioutil.ReadAll(resp.Body)
			bodyErr := fmt.Errorf("error posting results (%v): %v",
				resp.StatusCode, string(body))
			if readAllErr != nil {
				return bodyErr
			}
			switch resp.StatusCode {
			case http.StatusOK:
				return nil
			case http.StatusBadRequest:
				return bodyErr
			default:
				return util.RetriableError{bodyErr}
			}
		},
	)
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, 10, 1)
	if retryFail {
		return fmt.Errorf("attaching task files failed after %v tries: %v", 10, err)
	}
	return nil
}

// TaskPostTestLog posts a test log for a communicator's task.
func (t *TaskJSONCommunicator) TaskPostTestLog(log *model.TestLog) (string, error) {
	var logId string
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.tryPostJSON("test_logs", log)
			if err != nil {
				err := fmt.Errorf("error posting logs: %v", err)
				if resp != nil {
					resp.Body.Close()
				}
				return util.RetriableError{err}
			}

			if resp.StatusCode == http.StatusOK {
				logReply := struct {
					Id string `json:"_id"`
				}{}
				err = util.ReadJSONInto(resp.Body, &logReply)
				if err != nil {
					return err
				}
				logId = logReply.Id
				return nil
			}
			bodyErr, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusBadRequest {
				return fmt.Errorf("bad request posting logs: %v", string(bodyErr))
			}
			return util.RetriableError{fmt.Errorf("failed posting logs: %v", string(bodyErr))}
		},
	)
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, 10, 1)
	if retryFail {
		return "", fmt.Errorf("attaching test logs failed after %v tries: %v", 10, err)
	}
	return logId, nil
}

// Start marks the communicator's task as started.
func (h *HTTPCommunicator) Start(pid string) error {
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pid}
	resp, retryFail, err := h.postJSON("start", taskStartRequest)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if retryFail {
			msg := fmt.Errorf("task start failed after %v tries: %v",
				h.MaxAttempts, err)
			h.Logger.Logf(slogger.ERROR, msg.Error())
			return msg
		} else {
			h.Logger.Logf(slogger.ERROR, "Failed to start task: %v", err)
			return err
		}
	}
	return nil
}

// End marks the communicator's task as finished with the given status.
func (h *HTTPCommunicator) End(status string,
	details *apimodels.TaskEndDetails) (*apimodels.TaskEndResponse, error) {
	taskEndReq := &apimodels.TaskEndRequest{Status: status}
	taskEndResp := &apimodels.TaskEndResponse{}
	if details != nil {
		taskEndReq.StatusDetails = *details
	}

	resp, retryFail, err := h.postJSON("end", taskEndReq)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if retryFail {
			errMsg := fmt.Errorf("task end failed after %v tries: %v",
				h.MaxAttempts, err)
			h.Logger.Logf(slogger.ERROR, errMsg.Error())
			return nil, errMsg
		} else {
			h.Logger.Logf(slogger.ERROR, "Failed to end task: %v", err)
			return nil, err
		}
	}

	if resp != nil {
		if err = util.ReadJSONInto(resp.Body, taskEndResp); err != nil {
			message := fmt.Sprintf("Error unmarshalling task end response: %v",
				err)
			h.Logger.Logf(slogger.ERROR, message)
			return nil, fmt.Errorf(message)
		}
		if resp.StatusCode != http.StatusOK {
			message := fmt.Sprintf("unexpected status code in task end "+
				"request (%v): %v", resp.StatusCode, taskEndResp.Message)
			return nil, fmt.Errorf(message)
		}
		err = nil
	} else {
		err = fmt.Errorf("received nil response from API server")
	}
	return taskEndResp, err
}

// Log sends a batch of log messages for the task's logs to the API server.
func (h *HTTPCommunicator) Log(messages []model.LogMessage) error {
	outgoingData := model.TaskLog{
		TaskId:       h.TaskId,
		Timestamp:    time.Now(),
		MessageCount: len(messages),
		Messages:     messages,
	}

	resp, err := h.tryPostJSON("log", outgoingData)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	return nil
}

// GetPatch loads the task's patch diff from the API server.
func (h *HTTPCommunicator) GetPatch() (*patch.Patch, error) {
	patch := &patch.Patch{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("patch")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, patch)
				if err != nil {
					return util.RetriableError{err}
				}
				return nil
			}
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting patch failed after %v tries: %v",
			h.MaxAttempts, err)
	}
	return patch, nil
}

// GetTask returns the communicator's task.
func (h *HTTPCommunicator) GetTask() (*model.Task, error) {
	task := &model.Task{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, task)
				if err != nil {
					fmt.Printf("error3, retrying: %v\n", err)
					return util.RetriableError{err}
				}
				return nil
			}
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting task failed after %v tries: %v",
			h.MaxAttempts, err)
	}
	return task, nil
}

// GetDistro returns the distro for the communicator's task.
func (h *HTTPCommunicator) GetDistro() (*distro.Distro, error) {
	d := &distro.Distro{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("distro")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, d)
				if err != nil {
					h.Logger.Errorf(slogger.ERROR,
						"unable to read distro response: %v\n", err)
					return util.RetriableError{err}
				}
				return nil
			}
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting distro failed after %v tries: %v", h.MaxAttempts, err)
	}
	return d, nil
}

// GetProjectConfig loads the communicator's task's project from the API server.
func (h *HTTPCommunicator) GetProjectRef() (*model.ProjectRef, error) {
	projectRef := &model.ProjectRef{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("project_ref")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, projectRef)
				if err != nil {
					return util.RetriableError{err}
				}
				return nil
			}
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting project ref failed after %v tries: %v", h.MaxAttempts, err)
	}
	return projectRef, nil
}

// GetProjectConfig loads the communicator's task's project from the API server.
func (h *HTTPCommunicator) GetProjectConfig() (*model.Project, error) {
	projectConfig := &model.Project{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("version")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				v := &version.Version{}
				err = util.ReadJSONInto(resp.Body, v)
				if err != nil {
					h.Logger.Errorf(slogger.ERROR,
						"unable to read project version response: %v\n", err)
					return util.RetriableError{fmt.Errorf("unable to read "+
						"project version response: %v\n", err)}
				}
				err = model.LoadProjectInto([]byte(v.Config), projectConfig)
				if err != nil {
					h.Logger.Errorf(slogger.ERROR,
						"unable to unmarshal project config: %v\n", err)
					return util.RetriableError{fmt.Errorf("unable to "+
						"unmarshall project config: %v\n", err)}
				}
				return nil
			}
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting project configuration failed after %v "+
			"tries: %v", h.MaxAttempts, err)
	}
	return projectConfig, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// and "abort" response. This function returns true if the agent should abort.
func (h *HTTPCommunicator) Heartbeat() (bool, error) {
	h.Logger.Logf(slogger.INFO, "Sending heartbeat.")
	resp, err := h.tryPostJSON("heartbeat", "heartbeat")
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		h.Logger.Logf(slogger.ERROR, "Error sending heartbeat: %v", err)
		return false, err
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code doing heartbeat: %v",
			resp.StatusCode)
	}
	if resp != nil && resp.StatusCode == http.StatusConflict {
		h.Logger.Logf(slogger.ERROR, "wrong secret (409) sending heartbeat")
		h.SignalChan <- IncorrectSecret
		return false, fmt.Errorf("unauthorized - wrong secret")
	}

	heartbeatResponse := &apimodels.HeartbeatResponse{}
	if err = util.ReadJSONInto(resp.Body, heartbeatResponse); err != nil {
		h.Logger.Logf(slogger.ERROR, "Error unmarshaling heartbeat "+
			"response: %v", err)
		return false, err
	}
	return heartbeatResponse.Abort, nil
}

func (h *HTTPCommunicator) tryGet(path string) (*http.Response, error) {
	return h.tryRequest(path, "GET", nil)
}

func (h *HTTPCommunicator) tryPostJSON(path string, data interface{}) (
	*http.Response, error) {
	return h.tryRequest(path, "POST", &data)
}

func (h *HTTPCommunicator) tryRequest(path string, method string,
	data *interface{}) (*http.Response, error) {
	endpointUrl := fmt.Sprintf("%s/task/%s/%s", h.ServerURLRoot, h.TaskId,
		path)
	req, err := http.NewRequest(method, endpointUrl, nil)
	if err != nil {
		return nil, err
	}

	if data != nil {
		jsonBytes, err := json.Marshal(*data)
		if err != nil {
			return nil, err
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))
	}
	req.Header.Add(evergreen.TaskSecretHeader, h.TaskSecret)
	req.Header.Add("Content-Type", "application/json")
	return h.httpClient.Do(req)
}

func (h *HTTPCommunicator) postJSON(path string, data interface{}) (
	resp *http.Response, retryFail bool, err error) {
	retriablePost := util.RetriableFunc(
		func() error {
			resp, err = h.tryPostJSON(path, data)
			if err == nil && resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				h.Logger.Logf(slogger.ERROR, "received 409 conflict error")
				return HTTPConflictError
			}
			if err != nil {
				h.Logger.Logf(slogger.ERROR, "HTTP Post failed on '%v': %v",
					path, err)
				return util.RetriableError{err}
			} else {
				h.Logger.Logf(slogger.ERROR, "bad response '%v' posting to "+
					"'%v'", resp.StatusCode, path)
				return util.RetriableError{fmt.Errorf("unexpected status "+
					"code: %v", resp.StatusCode)}
			}
		},
	)
	retryFail, err = util.Retry(retriablePost, h.MaxAttempts, h.RetrySleep)
	return resp, retryFail, err
}

// FetchExpansionVars loads expansions for a communicator's task from the API server.
func (h *HTTPCommunicator) FetchExpansionVars() (*apimodels.ExpansionVars, error) {
	resultVars := &apimodels.ExpansionVars{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("fetch_vars")
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				// Some generic error trying to connect - try again
				h.Logger.Logf(slogger.ERROR, "failed trying to call fetch GET: %v", err)
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusUnauthorized {
				err = fmt.Errorf("fetching expansions failed: got 'unauthorized' response.")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return err
			}
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("failed trying fetch GET, got bad response code: %v", resp.StatusCode)
				h.Logger.Logf(slogger.ERROR, err.Error())
				return util.RetriableError{err}
			}
			if resp == nil {
				err = fmt.Errorf("empty response fetching expansions")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return util.RetriableError{err}
			}

			// got here safely, so all is good - read the results
			err = util.ReadJSONInto(resp.Body, resultVars)
			if err != nil {
				err = fmt.Errorf("failed to read vars from response: %v", err)
				h.Logger.Logf(slogger.ERROR, err.Error())
				return err
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, 10, 1*time.Second)
	if err != nil {
		// stop trying to make fetch happen, it's not going to happen
		if retryFail {
			h.Logger.Logf(slogger.ERROR, "Fetching vars used up all retries.")
		}
		return nil, err
	}
	return resultVars, err
}
