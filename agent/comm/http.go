package comm

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

const httpMaxAttempts = 10

var HeartbeatTimeout = time.Minute

var HTTPConflictError = errors.New("Conflict")

// HTTPCommunicator handles communication with the API server. An HTTPCommunicator
// is scoped to a single task, and all communication performed by it is
// only relevant to that running task.
type HTTPCommunicator struct {
	ServerURLRoot string
	TaskId        string
	TaskSecret    string
	HostId        string
	HostSecret    string
	MaxAttempts   int
	RetrySleep    time.Duration
	SignalChan    chan Signal
	Logger        *slogger.Logger
	HttpsCert     string
	httpClient    *http.Client
	// TODO only use one Client after global locking is removed
	heartbeatClient *http.Client
}

// NewHTTPCommunicator returns an initialized HTTPCommunicator.
// The cert parameter may be blank if default system certificates are being used.
func NewHTTPCommunicator(serverURL, hostId, hostSecret, cert string) (*HTTPCommunicator, error) {
	agentCommunicator := &HTTPCommunicator{
		ServerURLRoot: fmt.Sprintf("%v/api/%v", serverURL, evergreen.AgentAPIVersion),
		HostId:        hostId,
		HostSecret:    hostSecret,
		MaxAttempts:   httpMaxAttempts,
		RetrySleep:    time.Second * 2,
		HttpsCert:     cert,
	}

	if agentCommunicator.HttpsCert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(agentCommunicator.HttpsCert)) {
			return nil, errors.New("failed to append HttpsCert to new cert pool")
		}
		tc := &tls.Config{RootCAs: pool}
		tr := &http.Transport{TLSClientConfig: tc}
		agentCommunicator.httpClient = &http.Client{Transport: tr}
		agentCommunicator.heartbeatClient = &http.Client{Transport: tr, Timeout: HeartbeatTimeout}
	} else {
		agentCommunicator.httpClient = &http.Client{}
		agentCommunicator.heartbeatClient = &http.Client{Timeout: HeartbeatTimeout}
	}
	return agentCommunicator, nil
}

func (h *HTTPCommunicator) SetSignalChan(communicatorChan chan Signal) {
	h.SignalChan = communicatorChan
}

func (h *HTTPCommunicator) SetLogger(logger *slogger.Logger) {
	h.Logger = logger
}

// Heartbeat encapsulates heartbeat behavior (i.e., pinging the API server at regular
// intervals to ensure that communication hasn't broken down).
type Heartbeat interface {
	Heartbeat() (bool, error)
}

// Start marks the communicator's task as started.
func (h *HTTPCommunicator) Start() error {
	pidStr := strconv.Itoa(os.Getpid())
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pidStr}
	resp, retryFail, err := h.postJSON("start", taskStartRequest)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if retryFail {
			err = errors.Wrapf(err, "task start failed after %v tries", h.MaxAttempts)
		} else {
			err = errors.Wrap(err, "failed to start task")
		}
		h.Logger.Logf(slogger.ERROR, err.Error())
		return err
	}
	return nil
}

// End marks the communicator's task as finished with the given status.
func (h *HTTPCommunicator) End(detail *apimodels.TaskEndDetail) (*apimodels.EndTaskResponse, error) {
	taskEndResp := &apimodels.EndTaskResponse{}
	resp, retryFail, err := h.postJSON("end", detail)
	if resp == nil {
		return nil, errors.New("empty response when trying to end task")
	}
	defer resp.Body.Close()
	if err != nil {
		if retryFail {
			var bodyMsg []byte
			if resp != nil {
				bodyMsg, _ = ioutil.ReadAll(resp.Body)
			}
			err = errors.Wrapf(err, "task end failed after %v tries: %v", h.MaxAttempts, bodyMsg)
		} else {
			err = errors.Wrap(err, "failed to end task")
		}
		h.Logger.Logf(slogger.ERROR, err.Error())
		return nil, err
	}

	if resp != nil {
		if err = util.ReadJSONInto(resp.Body, taskEndResp); err != nil {
			message := fmt.Sprintf("Error unmarshalling task end response: %v",
				err)
			h.Logger.Logf(slogger.ERROR, message)
			return nil, errors.New(message)
		}
		if resp.StatusCode != http.StatusOK {
			message := fmt.Sprintf("unexpected status code in task end "+
				"request (%v): %v", resp.StatusCode, taskEndResp.Message)
			return nil, errors.New(message)
		}
		err = nil
	} else {
		err = errors.New("received nil response from API server")
	}
	h.Logger.Logf(slogger.INFO, "task's end response received: %s", taskEndResp.Message)
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

	retriableLog := util.RetriableFunc(
		func() error {
			resp, err := h.TryTaskPost("log", outgoingData)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				return util.RetriableError{errors.WithStack(err)}
			}
			if resp.StatusCode == http.StatusInternalServerError {
				return util.RetriableError{errors.Errorf("http status %v response body %v", resp.StatusCode, resp.Body)}
			}
			return nil
		},
	)
	retryFail, err := util.Retry(retriableLog, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return errors.Wrapf(err, "logging failed after %vtries: %v", h.MaxAttempts)
	}
	return err
}

// GetTask returns the communicator's task.
func (h *HTTPCommunicator) GetTask() (*task.Task, error) {
	task := &task.Task{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryTaskGet("")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return errors.New("conflict; wrong secret")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{errors.New("empty response")}
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
		return nil, errors.Wrapf(err, "getting task failed after %v tries", h.MaxAttempts)
	}
	return task, nil
}

// GetDistro returns the distro for the communicator's task.
func (h *HTTPCommunicator) GetDistro() (*distro.Distro, error) {
	d := &distro.Distro{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryTaskGet("distro")
			if resp == nil {
				return util.RetriableError{errors.New("empty response")}
			}

			defer resp.Body.Close()
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}

			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return errors.New("conflict; wrong secret")
			}
			if resp.StatusCode != http.StatusOK {
				return util.RetriableError{errors.Errorf("bad status: %s", resp.Status)}
			}

			err = util.ReadJSONInto(resp.Body, d)
			if err != nil {
				err = errors.Wrap(err, "unable to read distro response")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return util.RetriableError{err}
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, errors.Wrapf(err, "getting distro failed after %d tries", h.MaxAttempts)
	}
	return d, nil
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (h *HTTPCommunicator) GetNextTask() (*apimodels.NextTaskResponse, error) {
	taskResponse := &apimodels.NextTaskResponse{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryGet("agent/next_task")
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				return util.RetriableError{err}
			}
			err = util.ReadJSONInto(resp.Body, taskResponse)
			if err != nil {
				return util.RetriableError{err}
			}
			return nil
		})
	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, fmt.Errorf("getting next task failed after %v tries: %v", h.MaxAttempts, err)
	}
	return taskResponse, nil

}

// GetProjectConfig loads the communicator's task's project from the API server.
func (h *HTTPCommunicator) GetProjectRef() (*model.ProjectRef, error) {
	projectRef := &model.ProjectRef{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryTaskGet("project_ref")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				// Something very wrong, fail now with no retry.
				return errors.New("conflict; wrong secret")
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{errors.New("empty response")}
			}

			err = util.ReadJSONInto(resp.Body, projectRef)
			if err != nil {
				return util.RetriableError{err}
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, errors.Wrapf(err, "getting project ref failed after %d tries", h.MaxAttempts)
	}
	return projectRef, nil
}

// GetVersion loads the communicator's task's version from the API server.
func (h *HTTPCommunicator) GetVersion() (*version.Version, error) {
	v := &version.Version{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryTaskGet("version")
			if resp != nil {
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusConflict {
					// Something very wrong, fail now with no retry.
					return errors.New("conflict; wrong secret")
				}
				if resp.StatusCode != http.StatusOK {
					msg, _ := ioutil.ReadAll(resp.Body) // ignore ReadAll error
					return util.RetriableError{
						errors.Errorf("bad status code %v: %s",
							resp.StatusCode, string(msg)),
					}
				}
			}

			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{errors.WithStack(err)}
			}

			if resp == nil {
				return util.RetriableError{errors.New("empty response")}
			}

			err = util.ReadJSONInto(resp.Body, v)
			if err != nil {
				err = errors.Wrap(err, "unable to read project version response")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return err
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return nil, errors.Wrapf(err, "getting project configuration failed after %d tries",
			h.MaxAttempts)
	}
	return v, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// and "abort" response. This function returns true if the agent should abort.
func (h *HTTPCommunicator) Heartbeat() (bool, error) {
	h.Logger.Logf(slogger.INFO, "Sending heartbeat.")
	data := interface{}("heartbeat")
	resp, err := h.tryRequestWithClient(h.getTaskPath("heartbeat"), "POST", h.heartbeatClient, &data)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		err = errors.Wrap(err, "error sending heartbeat")
		h.Logger.Logf(slogger.ERROR, err.Error())
		return false, err
	}
	if resp.StatusCode == http.StatusConflict {
		h.Logger.Logf(slogger.ERROR, "wrong secret (409) sending heartbeat")
		h.SignalChan <- IncorrectSecret
		return false, errors.Errorf("unauthorized - wrong secret")
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.Errorf("unexpected status code doing heartbeat: %v",
			resp.StatusCode)
	}

	heartbeatResponse := &apimodels.HeartbeatResponse{}
	if err = util.ReadJSONInto(resp.Body, heartbeatResponse); err != nil {
		err = errors.Wrap(err, "Error unmarshaling heartbeat response")
		h.Logger.Logf(slogger.ERROR, err.Error())
		return false, err
	}
	return heartbeatResponse.Abort, nil
}

func (h *HTTPCommunicator) SetTask(taskId, taskSecret string) {
	h.TaskId = taskId
	h.TaskSecret = taskSecret
}

func (h *HTTPCommunicator) GetCurrentTaskId() string {
	return h.TaskId
}

func (h *HTTPCommunicator) Reset(commSignal chan Signal, timeoutWatcher *TimeoutWatcher) (*APILogger, *StreamLogger, error) {

	h.SignalChan = commSignal
	// set up logger to API server
	apiLogger := NewAPILogger(h)

	// set up timeout logger, local and API logger streams
	streamLogger, err := NewStreamLogger(timeoutWatcher, apiLogger)
	if err != nil {
		return nil, nil, err
	}
	h.Logger = streamLogger.Execution
	return apiLogger, streamLogger, nil

}

// getTaskPath is a helper to create a path that can be used for task specific calls
func (h *HTTPCommunicator) getTaskPath(path string) string {
	return fmt.Sprintf("task/%v/%v", h.TaskId, path)
}

func (h *HTTPCommunicator) TryGet(path string) (*http.Response, error) {
	resp, err := h.tryRequestWithClient(path, "GET", h.httpClient, nil)
	return resp, errors.WithStack(err)
}

func (h *HTTPCommunicator) TryTaskGet(path string) (*http.Response, error) {
	resp, err := h.tryRequestWithClient(h.getTaskPath(path), "GET", h.httpClient, nil)
	return resp, errors.WithStack(err)
}

func (h *HTTPCommunicator) TryTaskPost(path string, data interface{}) (*http.Response, error) {
	resp, err := h.tryRequestWithClient(h.getTaskPath(path), "POST", h.httpClient, &data)
	return resp, errors.WithStack(err)
}

func (h *HTTPCommunicator) TryPostJSON(path string, data interface{}) (*http.Response, error) {
	resp, err := h.tryRequestWithClient(path, "POST", h.httpClient, &data)
	return resp, errors.WithStack(err)
}

// tryRequestWithClient does the given task HTTP request using the provided client, allowing
// requests to be done with multiple client configurations/timeouts.
func (h *HTTPCommunicator) tryRequestWithClient(path string, method string, client *http.Client,
	data *interface{}) (*http.Response, error) {
	endpointUrl := fmt.Sprintf("%s/%s", h.ServerURLRoot, path)
	req, err := http.NewRequest(method, endpointUrl, nil)
	err = errors.WithStack(err)
	if err != nil {
		return nil, err
	}

	if data != nil {
		var out []byte
		out, err = json.Marshal(*data)
		if err != nil {
			return nil, err
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(out))
	}
	req.Header.Add(evergreen.TaskSecretHeader, h.TaskSecret)
	req.Header.Add(evergreen.HostHeader, h.HostId)
	req.Header.Add(evergreen.HostSecretHeader, h.HostSecret)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	return resp, errors.WithStack(err)
}

func (h *HTTPCommunicator) postJSON(path string, data interface{}) (
	resp *http.Response, retryFail bool, err error) {
	retriablePost := util.RetriableFunc(
		func() error {
			resp, err = h.TryTaskPost(path, data)
			if resp == nil {
				h.Logger.Logf(slogger.ERROR, "nil response")
				return errors.New("response is nil")
			}
			if err != nil {
				h.Logger.Logf(slogger.ERROR, "HTTP Post failed on '%v': %v",
					path, err)
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp.StatusCode == http.StatusConflict {
				h.Logger.Logf(slogger.ERROR, "received 409 conflict error")
				return HTTPConflictError
			}
			h.Logger.Logf(slogger.ERROR, "bad response '%v' posting to "+
				"'%v'", resp.StatusCode, path)
			err = errors.Errorf("unexpected status code: %v", resp.StatusCode)
			return util.RetriableError{err}
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
			resp, err := h.TryTaskGet("fetch_vars")
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				// Some generic error trying to connect - try again
				h.Logger.Logf(slogger.ERROR, "failed trying to call fetch GET: %v", err)
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusUnauthorized {
				err = errors.Errorf("fetching expansions failed: got 'unauthorized' response.")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return err
			}
			if resp.StatusCode != http.StatusOK {
				err = errors.Errorf("failed trying fetch GET, got bad response code: %v", resp.StatusCode)
				h.Logger.Logf(slogger.ERROR, err.Error())
				return util.RetriableError{err}
			}
			if resp == nil {
				err = errors.New("empty response fetching expansions")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return util.RetriableError{err}
			}

			// got here safely, so all is good - read the results
			err = util.ReadJSONInto(resp.Body, resultVars)
			if err != nil {
				err = errors.Wrap(err, "failed to read vars from response")
				h.Logger.Logf(slogger.ERROR, err.Error())
				return err
			}
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, httpMaxAttempts, 1*time.Second)
	err = errors.WithStack(err)
	if err != nil {
		// stop trying to make fetch happen, it's not going to happen
		if retryFail {
			h.Logger.Logf(slogger.ERROR, "Fetching vars used up all retries.")
		}
		return nil, err
	}
	return resultVars, err
}
