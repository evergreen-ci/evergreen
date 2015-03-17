package agent

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/model"
	"10gen.com/mci/model/artifact"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/version"
	"10gen.com/mci/util"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"io/ioutil"
	"net/http"
	"time"
)

var HTTPConflictError = errors.New("Conflict")

type HTTPAgentCommunicator struct {
	ServerURLRoot string
	TaskId        string
	TaskSecret    string
	MaxAttempts   int
	RetrySleep    time.Duration
	SignalChan    chan AgentSignal
	Logger        *slogger.Logger
	HttpsCert     string
	httpClient    *http.Client
}

func NewHTTPAgentCommunicator(rootUrl string, taskId string, taskSecret string,
	cert string) (*HTTPAgentCommunicator, error) {

	taskCom := &HTTPAgentCommunicator{
		ServerURLRoot: fmt.Sprintf("%v/api/%v", rootUrl, APIVersion),
		TaskId:        taskId,
		TaskSecret:    taskSecret,
		MaxAttempts:   10,
		RetrySleep:    time.Second * 3,
		HttpsCert:     cert,
	}

	if taskCom.HttpsCert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(taskCom.HttpsCert)) {
			return nil, errors.New("failed to append HttpsCert to new cert pool")
		}
		tc := &tls.Config{RootCAs: pool}
		tr := &http.Transport{TLSClientConfig: tc}
		taskCom.httpClient = &http.Client{Transport: tr}
	} else {
		taskCom.httpClient = &http.Client{}
	}
	return taskCom, nil
}

type Heartbeat interface {
	Heartbeat() (bool, error)
}

type TaskJSONCommunicator struct {
	PluginName string
	TaskCommunicator
}

func (t *TaskJSONCommunicator) TaskPostJSON(endpoint string,
	data interface{}) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s", t.PluginName, endpoint)
	return t.tryPostJSON(uri, data)
}

func (t *TaskJSONCommunicator) TaskGetJSON(endpoint string) (*http.Response,
	error) {
	uri := fmt.Sprintf("%s/%s", t.PluginName, endpoint)
	return t.tryGet(uri)
}

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

// Used by PluginCommunicator interface for POST method for attaching task files
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

func (h *HTTPAgentCommunicator) Start(pid string) error {
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

func (h *HTTPAgentCommunicator) End(status string,
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

func (h *HTTPAgentCommunicator) Log(messages []model.LogMessage) error {
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

//TODO log errors in this function
func (h *HTTPAgentCommunicator) GetPatch() (*model.Patch, error) {
	patch := &model.Patch{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("patch")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				//Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				//Some generic error trying to connect - try again
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

//TODO log errors in this function
func (h *HTTPAgentCommunicator) GetTask() (*model.Task, error) {
	task := &model.Task{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				//Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				//Some generic error trying to connect - try again
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

func (h *HTTPAgentCommunicator) GetDistro() (*distro.Distro, error) {
	distro := &distro.Distro{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("distro")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				//Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				//Some generic error trying to connect - try again
				return util.RetriableError{err}
			}
			if resp == nil {
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, distro)
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
		return nil, fmt.Errorf("getting distro failed after %v tries: %v",
			h.MaxAttempts, err)
	}
	return distro, nil
}

func (h *HTTPAgentCommunicator) GetProjectConfig() (*model.Project, error) {
	projectConfig := &model.Project{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("version")
			if resp != nil {
				defer resp.Body.Close()
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				//Something very wrong, fail now with no retry.
				return fmt.Errorf("conflict - wrong secret!")
			}
			if err != nil {
				//Some generic error trying to connect - try again
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

func (h *HTTPAgentCommunicator) Heartbeat() (bool, error) {
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

func (h *HTTPAgentCommunicator) tryGet(path string) (*http.Response, error) {
	return h.tryRequest(path, "GET", nil)
}

func (h *HTTPAgentCommunicator) tryPostJSON(path string, data interface{}) (
	*http.Response, error) {
	return h.tryRequest(path, "POST", &data)
}

func (h *HTTPAgentCommunicator) tryRequest(path string, method string,
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
	req.Header.Add(mci.TaskSecretHeader, h.TaskSecret)
	req.Header.Add("Content-Type", "application/json")
	return h.httpClient.Do(req)
}

func (h *HTTPAgentCommunicator) postJSON(path string, data interface{}) (
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

func (h *HTTPAgentCommunicator) FetchExpansionVars() (*apimodels.ExpansionVars, error) {
	resultVars := &apimodels.ExpansionVars{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := h.tryGet("fetch_vars")
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
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

			//got here safely, so all is good - read the results
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
		//stop trying to make fetch happen, it's not going to happen
		if retryFail {
			h.Logger.Logf(slogger.ERROR, "Fetching vars used up all retries.")
		}
		return nil, err
	}
	return resultVars, err
}
