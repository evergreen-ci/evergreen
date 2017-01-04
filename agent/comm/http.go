package comm

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/tychoish/grip/slogger"
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
func NewHTTPCommunicator(serverURL, taskId, taskSecret, hostId, hostSecret, cert string, sigChan chan Signal) (*HTTPCommunicator, error) {
	agentCommunicator := &HTTPCommunicator{
		ServerURLRoot: fmt.Sprintf("%v/api/%v", serverURL, evergreen.AgentAPIVersion),
		TaskId:        taskId,
		TaskSecret:    taskSecret,
		HostId:        hostId,
		HostSecret:    hostSecret,
		MaxAttempts:   httpMaxAttempts,
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
		agentCommunicator.heartbeatClient = &http.Client{Transport: tr, Timeout: HeartbeatTimeout}
	} else {
		agentCommunicator.httpClient = &http.Client{}
		agentCommunicator.heartbeatClient = &http.Client{Timeout: HeartbeatTimeout}
	}
	return agentCommunicator, nil
}

// Heartbeat encapsulates heartbeat behavior (i.e., pinging the API server at regular
// intervals to ensure that communication hasn't broken down).
type Heartbeat interface {
	Heartbeat() (bool, error)
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
func (h *HTTPCommunicator) End(detail *apimodels.TaskEndDetail) (*apimodels.TaskEndResponse, error) {
	taskEndResp := &apimodels.TaskEndResponse{}
	resp, retryFail, err := h.postJSON("end", detail)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if retryFail {
			var bodyMsg []byte
			if resp != nil {
				bodyMsg, _ = ioutil.ReadAll(resp.Body)
			}
			errMsg := fmt.Errorf("task end failed after %v tries (%v): %v",
				h.MaxAttempts, err, bodyMsg)
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

	retriableLog := util.RetriableFunc(
		func() error {
			resp, err := h.TryPostJSON("log", outgoingData)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusInternalServerError {
				return util.RetriableError{fmt.Errorf("http status %v response body %v", resp.StatusCode, resp.Body)}
			}
			return nil
		},
	)
	retryFail, err := util.Retry(retriableLog, h.MaxAttempts, h.RetrySleep)
	if retryFail {
		return fmt.Errorf("logging failed after %vtries: %v",
			h.MaxAttempts, err)
	}
	return err
}

// GetTask returns the communicator's task.
func (h *HTTPCommunicator) GetTask() (*task.Task, error) {
	task := &task.Task{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryGet("")
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
			resp, err := h.TryGet("distro")
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
			resp, err := h.TryGet("project_ref")
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

// GetVersion loads the communicator's task's version from the API server.
func (h *HTTPCommunicator) GetVersion() (*version.Version, error) {
	v := &version.Version{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.TryGet("version")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil {
				if resp.StatusCode == http.StatusConflict {
					// Something very wrong, fail now with no retry.
					return fmt.Errorf("conflict - wrong secret!")
				}
				if resp.StatusCode != http.StatusOK {
					msg, _ := ioutil.ReadAll(resp.Body) // ignore ReadAll error
					return util.RetriableError{
						fmt.Errorf("bad status code %v: %v", resp.StatusCode, string(msg)),
					}
				}
			}
			if err != nil {
				// Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, v)
				if err != nil {
					h.Logger.Errorf(slogger.ERROR,
						"unable to read project version response: %v\n", err)
					return fmt.Errorf("unable to read project version response: %v\n", err)
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
	return v, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// and "abort" response. This function returns true if the agent should abort.
func (h *HTTPCommunicator) Heartbeat() (bool, error) {
	h.Logger.Logf(slogger.INFO, "Sending heartbeat.")
	data := interface{}("heartbeat")
	resp, err := h.tryRequestWithClient("heartbeat", "POST", h.heartbeatClient, &data)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		h.Logger.Logf(slogger.ERROR, "Error sending heartbeat: %v", err)
		return false, err
	}
	if resp.StatusCode == http.StatusConflict {
		h.Logger.Logf(slogger.ERROR, "wrong secret (409) sending heartbeat")
		h.SignalChan <- IncorrectSecret
		return false, fmt.Errorf("unauthorized - wrong secret")
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code doing heartbeat: %v",
			resp.StatusCode)
	}

	heartbeatResponse := &apimodels.HeartbeatResponse{}
	if err = util.ReadJSONInto(resp.Body, heartbeatResponse); err != nil {
		h.Logger.Logf(slogger.ERROR, "Error unmarshaling heartbeat "+
			"response: %v", err)
		return false, err
	}
	return heartbeatResponse.Abort, nil
}

func (h *HTTPCommunicator) TryGet(path string) (*http.Response, error) {
	return h.tryRequestWithClient(path, "GET", h.httpClient, nil)
}

func (h *HTTPCommunicator) TryPostJSON(path string, data interface{}) (
	*http.Response, error) {
	return h.tryRequestWithClient(path, "POST", h.httpClient, &data)
}

// tryRequestWithClient does the given task HTTP request using the provided client, allowing
// requests to be done with multiple client configurations/timeouts.
func (h *HTTPCommunicator) tryRequestWithClient(path string, method string, client *http.Client,
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
	req.Header.Add(evergreen.HostHeader, h.HostId)
	req.Header.Add(evergreen.HostSecretHeader, h.HostSecret)
	req.Header.Add("Content-Type", "application/json")
	return client.Do(req)
}

func (h *HTTPCommunicator) postJSON(path string, data interface{}) (
	resp *http.Response, retryFail bool, err error) {
	retriablePost := util.RetriableFunc(
		func() error {
			resp, err = h.TryPostJSON(path, data)
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
			resp, err := h.TryGet("fetch_vars")
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

	retryFail, err := util.Retry(retriableGet, httpMaxAttempts, 1*time.Second)
	if err != nil {
		// stop trying to make fetch happen, it's not going to happen
		if retryFail {
			h.Logger.Logf(slogger.ERROR, "Fetching vars used up all retries.")
		}
		return nil, err
	}
	return resultVars, err
}
