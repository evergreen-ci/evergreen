package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func getBFSuggestionClient(bbProj evergreen.BuildBaronProject) *bfSuggestionClient {
	if bbProj.BFSuggestionServer == "" {
		return nil
	}
	return &bfSuggestionClient{
		bbProj.BFSuggestionServer,
		bbProj.BFSuggestionUsername,
		bbProj.BFSuggestionPassword}
}

const suggestionPath = "/suggestions/{task_id}/{execution}"
const getFeedbackPath = "/feedback/{task_id}/{execution}/user_id/{user_id}"
const sendFeedbackPath = "/feedback/{task_id}/{execution}"
const removeFeedbackPath = "/feedback/{task_id}/{execution}/user_id/{user_id}/feedback_type/{feedback_type}"

type bfSuggestionClient struct {
	server   string
	username string
	password string
}

// Send a request to the BF Suggestion server.
func (bfsc *bfSuggestionClient) request(ctx context.Context, req *http.Request) ([]byte, error) {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	if bfsc.username != "" {
		req.SetBasicAuth(bfsc.username, bfsc.password)
	}

	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read HTTP response body (status: %s)", resp.Status)
	}

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return nil, errors.Errorf("HTTP request returned unexpected status: %s (body: %s)", resp.Status, string(body))
	}
	return body, nil
}

// Send a GET request to the BF Suggestion server.
func (bfsc *bfSuggestionClient) get(ctx context.Context, url string) ([]byte, error) {
	var req *http.Request
	var err error

	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return bfsc.request(ctx, req)
}

// Send a POST request to the BF Suggestion server.
func (bfsc *bfSuggestionClient) post(ctx context.Context, url string, data interface{}) ([]byte, error) {
	var body io.Reader = nil
	if data != nil {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to encode the request body into JSON")
		}
		body = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	return bfsc.request(ctx, req)
}

// Send a DELETE request to the BF Suggestion server.
func (bfsc *bfSuggestionClient) delete(ctx context.Context, url string) ([]byte, error) {
	var req *http.Request
	var err error

	req, err = http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	return bfsc.request(ctx, req)
}

type bfSuggestion struct {
	TestName string `json:"test_name"`
	Issues   []struct {
		Key         string `json:"key"`
		Summary     string `json:"summary"`
		Status      string `json:"status"`
		Resolution  string `json:"resolution"`
		CreatedDate string `json:"created_date"`
		UpdatedDate string `json:"updated_date"`
	}
}

type bfSuggestionResponse struct {
	Status      string         `json:"status"`
	Suggestions []bfSuggestion `json:"suggestions"`
}

// Retrieve suggestions from the BF Suggestion server.
func (bfsc *bfSuggestionClient) getSuggestions(ctx context.Context, t *task.Task) (bfSuggestionResponse, error) {
	data := bfSuggestionResponse{}

	url := bfsc.server + suggestionPath
	url = strings.Replace(url, "{task_id}", t.Id, -1)
	url = strings.Replace(url, "{execution}", strconv.Itoa(t.Execution), -1)

	body, err := bfsc.get(ctx, url)
	if err != nil {
		return data, err
	}

	if err = json.Unmarshal(body, &data); err != nil {
		return data, errors.Wrap(err, "Failed to parse Build Baron suggestions")
	}
	if data.Status != "ok" {
		return data, errors.Errorf("Build Baron suggestions weren't ready: status=%s", data.Status)
	}
	return data, nil
}

type feedbackItem struct {
	UserId       string                 `json:"user_id"`
	FeedbackType string                 `json:"type"`
	FeedbackData map[string]interface{} `json:"data"`
}

type feedbackResponse struct {
	TaskId        string         `json:"task_id"`
	Execution     int            `json:"execution"`
	FeedbackItems []feedbackItem `json:"items"`
}

// Retrieve user feedback from the BF Suggestion server.
func (bfsc *bfSuggestionClient) getFeedback(ctx context.Context, t *task.Task, userId string) ([]feedbackItem, error) {
	data := feedbackResponse{}
	items := []feedbackItem{}

	url := bfsc.server + getFeedbackPath
	url = strings.Replace(url, "{task_id}", t.Id, -1)
	url = strings.Replace(url, "{execution}", strconv.Itoa(t.Execution), -1)
	url = strings.Replace(url, "{user_id}", userId, -1)

	body, err := bfsc.get(ctx, url)
	if err != nil {
		return items, err
	}

	if err = json.Unmarshal(body, &data); err != nil {
		return items, errors.Wrap(err, "Failed to parse Build Baron feedback")
	}
	return data.FeedbackItems, nil
}

// Send user feedback to the BF Suggestion server.
func (bfsc *bfSuggestionClient) sendFeedback(ctx context.Context, t *task.Task, userId string,
	feedbackType string, feedbackData map[string]interface{}) error {
	feedback := feedbackItem{userId, feedbackType, feedbackData}

	url := bfsc.server + sendFeedbackPath
	url = strings.Replace(url, "{task_id}", t.Id, -1)
	url = strings.Replace(url, "{execution}", strconv.Itoa(t.Execution), -1)

	_, err := bfsc.post(ctx, url, feedback)
	if err != nil {
		grip.Error(fmt.Sprintf("Failed to send feedback to BF Suggestion server (url: %s, data: %v): %s", url, feedback, err))
	}
	return err
}

// Remove user feedback from the BF Suggestion server.
func (bfsc *bfSuggestionClient) removeFeedback(ctx context.Context, t *task.Task, userId string,
	feedbackType string) error {
	url := bfsc.server + removeFeedbackPath
	url = strings.Replace(url, "{task_id}", t.Id, -1)
	url = strings.Replace(url, "{execution}", strconv.Itoa(t.Execution), -1)
	url = strings.Replace(url, "{user_id}", userId, -1)
	url = strings.Replace(url, "{feedback_type}", feedbackType, -1)

	_, err := bfsc.delete(ctx, url)
	if err != nil {
		grip.Error(fmt.Sprintf("Failed to delete user feedback from BF Suggestion server (url: %s): %s", url, err))
	}
	return err
}
