package comm

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
)

// TaskJSONCommunicator handles plugin-specific JSON-encoded
// communication with the API server.
type TaskJSONCommunicator struct {
	PluginName string
	TaskCommunicator
}

// TaskPostJSON does an HTTP POST for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskPostJSON(endpoint string, data interface{}) (*http.Response, error) {
	return t.TryPostJSON(fmt.Sprintf("%s/%s", t.PluginName, endpoint), data)
}

// TaskGetJSON does an HTTP GET for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskGetJSON(endpoint string) (*http.Response, error) {
	return t.TryGet(fmt.Sprintf("%s/%s", t.PluginName, endpoint))
}

// TaskPostResults posts a set of test results for the communicator's task.
func (t *TaskJSONCommunicator) TaskPostResults(results *task.TestResults) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryPostJSON("results", results)
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
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, httpMaxAttempts, 1)
	if retryFail {
		return fmt.Errorf("attaching test results failed after %v tries: %v", httpMaxAttempts, err)
	}
	return nil
}

// PostTaskFiles is used by the PluginCommunicator interface for attaching task files.
func (t *TaskJSONCommunicator) PostTaskFiles(task_files []*artifact.File) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryPostJSON("files", task_files)
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
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, httpMaxAttempts, 1)
	if retryFail {
		return fmt.Errorf("attaching task files failed after %v tries: %v", httpMaxAttempts, err)
	}
	return nil
}

// TaskPostTestLog posts a test log for a communicator's task.
func (t *TaskJSONCommunicator) TaskPostTestLog(log *model.TestLog) (string, error) {
	var logId string
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryPostJSON("test_logs", log)
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
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, httpMaxAttempts, 1)
	if err != nil {
		if retryFail {
			return "", fmt.Errorf("attaching test logs failed after %v tries: %v", httpMaxAttempts, err)
		}
		return "", fmt.Errorf("attaching test logs failed: %v", err)
	}
	return logId, nil
}
