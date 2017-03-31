package comm

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// TaskJSONCommunicator handles plugin-specific JSON-encoded
// communication with the API server.
type TaskJSONCommunicator struct {
	PluginName string
	TaskCommunicator
}

// TaskPostJSON does an HTTP POST for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskPostJSON(endpoint string, data interface{}) (*http.Response, error) {
	return t.TryTaskPost(fmt.Sprintf("%s/%s", t.PluginName, endpoint), data)
}

// TaskGetJSON does an HTTP GET for the communicator's plugin + task.
func (t *TaskJSONCommunicator) TaskGetJSON(endpoint string) (*http.Response, error) {
	return t.TryTaskGet(fmt.Sprintf("%s/%s", t.PluginName, endpoint))
}

// TaskPostResults posts a set of test results for the communicator's task.
func (t *TaskJSONCommunicator) TaskPostResults(results *task.TestResults) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryTaskPost("results", results)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				return util.RetriableError{errors.Wrap(err, "error posting results")}
			}

			body, _ := ioutil.ReadAll(resp.Body)
			bodyErr := errors.Errorf("error posting results (%v): %s",
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
		return errors.Wrapf(err, "attaching test results failed after %d tries", httpMaxAttempts)
	}
	return nil
}

// PostTaskFiles is used by the PluginCommunicator interface for attaching task files.
func (t *TaskJSONCommunicator) PostTaskFiles(task_files []*artifact.File) error {
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryTaskPost("files", task_files)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				return util.RetriableError{errors.Wrap(err, "error posting results")}
			}
			body, readAllErr := ioutil.ReadAll(resp.Body)
			bodyErr := errors.Errorf("error posting results (%v): %v",
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
		return errors.Wrapf(err, "attaching task files failed after %d tries", httpMaxAttempts)
	}
	return nil
}

// TaskPostTestLog posts a test log for a communicator's task.
func (t *TaskJSONCommunicator) TaskPostTestLog(log *model.TestLog) (string, error) {
	var logId string
	retriableSendFile := util.RetriableFunc(
		func() error {
			resp, err := t.TryTaskPost("test_logs", log)
			if resp != nil {
				defer resp.Body.Close()
			}

			if err != nil {
				return util.RetriableError{errors.Wrap(err, "error posting logs")}
			}

			if resp.StatusCode == http.StatusOK {
				logReply := struct {
					Id string `json:"_id"`
				}{}

				err = util.ReadJSONInto(resp.Body, &logReply)
				logId = logReply.Id
				return errors.WithStack(err)
			}

			bodyErr, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return util.RetriableError{errors.WithStack(err)}
			}

			if resp.StatusCode == http.StatusBadRequest {
				return errors.Errorf("bad request posting logs: %s", string(bodyErr))
			}

			return util.RetriableError{errors.Errorf("failed posting logs: %s", string(bodyErr))}
		},
	)
	retryFail, err := util.RetryArithmeticBackoff(retriableSendFile, httpMaxAttempts, 1)
	if err != nil {
		if retryFail {
			return "", errors.Wrapf(err, "attaching test logs failed after %v tries", httpMaxAttempts)
		}
		return "", errors.Wrap(err, "attaching test logs failed")
	}
	return logId, nil
}
