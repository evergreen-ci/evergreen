package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

func (*communicatorImpl) GetAllHosts() {}
func (*communicatorImpl) GetHostByID() {}

// GetHostsByUser will return a slice of all hosts spawned by the given user
// The API route is paginated, but we will add all pages to a local slice because
// there is an application-defined limit on the number of hosts a user can have
func (c *communicatorImpl) GetHostsByUser(ctx context.Context, user string) ([]*model.APIHost, error) {
	info := requestInfo{
		method:  get,
		path:    fmt.Sprintf("/users/%s/hosts", user),
		version: apiVersion2,
	}

	p, err := newPaginatorHelper(&info, c)
	if err != nil {
		return nil, err
	}

	hosts := []*model.APIHost{}
	for p.hasMore() {
		resp, err := p.getNextPage(ctx)
		if err != nil {
			return nil, err
		}

		temp := []*model.APIHost{}
		err = util.ReadJSONInto(resp.Body, &temp)
		if err != nil {
			err = resp.Body.Close()
			if err != nil {
				return nil, errors.Wrap(err, "error closing response body")
			}
			return nil, err
		}

		hosts = append(hosts, temp...)
	}
	return hosts, nil
}

func (*communicatorImpl) SetHostStatus()   {}
func (*communicatorImpl) SetHostStatuses() {}

// CreateSpawnHost will insert an intent host into the DB that will be spawned later by the runner
func (c *communicatorImpl) CreateSpawnHost(ctx context.Context, distroID string, keyName string) (*model.APIHost, error) {
	spawnRequest := &model.HostPostRequest{
		DistroID: distroID,
		KeyName:  keyName,
	}
	info := requestInfo{
		method:  post,
		path:    "hosts",
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, spawnRequest)
	if err != nil {
		err = errors.Wrapf(err, "error sending request to spawn host")
		return nil, err
	}

	defer resp.Body.Close()
	spawnHostResp := model.APIHost{}
	if err = util.ReadJSONInto(resp.Body, &spawnHostResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &spawnHostResp, nil
}

func (c *communicatorImpl) TerminateSpawnHost(ctx context.Context, hostID string) error {
	terminateRequest := &model.APISpawnHostModify{
		Action: "terminate",
		HostID: model.APIString(hostID),
	}
	info := requestInfo{
		method:  post,
		path:    "spawn",
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, terminateRequest)
	if err != nil {
		return errors.Wrapf(err, "error sending request to terminate host")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem terminating host and parsing error message")
		}
		return errors.Wrap(errMsg, "problem terminating host")
	}

	return nil
}

// GetHosts gathers all active hosts and invokes a function on them
func (c *communicatorImpl) GetHosts(ctx context.Context, f func([]*model.APIHost) error) error {
	info := requestInfo{
		method:  get,
		path:    "hosts",
		version: apiVersion2,
	}

	p, err := newPaginatorHelper(&info, c)
	if err != nil {
		return err
	}

	for p.hasMore() {
		hosts := []*model.APIHost{}
		resp, err := p.getNextPage(ctx)
		if err != nil {
			return err
		}

		err = util.ReadJSONInto(resp.Body, &hosts)
		if err != nil {
			return err
		}

		err = f(hosts)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *communicatorImpl) SetBannerMessage(ctx context.Context, message string, theme admin.BannerTheme) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin/banner",
	}

	_, err := c.retryRequest(ctx, info, struct {
		Banner string `json:"banner"`
		Theme  string `json:"theme"`
	}{
		Banner: message,
		Theme:  string(theme),
	})

	return errors.Wrap(err, "problem setting banner")
}

func (c *communicatorImpl) GetBannerMessage(ctx context.Context) (string, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    "admin",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "problem getting current banner message")
	}

	settings := model.APIAdminSettings{}
	if err = util.ReadJSONInto(resp.Body, &settings); err != nil {
		return "", errors.Wrap(err, "problem parsing response from server")
	}

	return string(settings.Banner), nil
}

func (c *communicatorImpl) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin/service_flags",
	}

	_, err := c.retryRequest(ctx, info, f)
	if err != nil {
		return errors.Wrap(err, "problem setting service flags")
	}

	return nil
}

func (c *communicatorImpl) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    "admin",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting service flags")
	}

	settings := model.APIAdminSettings{}
	if err = util.ReadJSONInto(resp.Body, &settings); err != nil {
		return nil, errors.Wrap(err, "problem parsing service flag response")
	}

	return &settings.ServiceFlags, nil
}

func (c *communicatorImpl) RestartRecentTasks(ctx context.Context, startAt, endAt time.Time) error {
	if endAt.Before(startAt) {
		return errors.Errorf("start (%s) cannot be before end (%s)", startAt, endAt)
	}

	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin/restart",
	}

	payload := struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		DryRun    bool      `json:"dry_run"`
	}{
		DryRun:    false,
		StartTime: startAt,
		EndTime:   endAt,
	}

	_, err := c.request(ctx, info, &payload)
	if err != nil {
		return errors.Wrap(err, "problem restarting recent tasks")
	}

	return nil
}

func (c *communicatorImpl) GetDistrosList(ctx context.Context) ([]model.APIDistro, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    "distros",
	}

	resp, client_err := c.request(ctx, info, "")
	if client_err != nil {
		return nil, errors.Wrap(client_err, "problem fetching distribution list")
	}
	defer resp.Body.Close()

	distros := []model.APIDistro{}

	err := util.ReadJSONInto(resp.Body, &distros)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing distribution list")
	}

	return distros, nil
}

func (c *communicatorImpl) GetCurrentUsersKeys(ctx context.Context) ([]model.APIPubKey, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    "keys",
	}

	resp, client_err := c.request(ctx, info, "")
	if client_err != nil {
		return nil, errors.Wrap(client_err, "problem fetching keys list")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}

		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return nil, errors.Wrap(err, "problem fetching key list and parsing error message")
		}
		return nil, errors.Wrap(errMsg, "problem fetching key list")
	}

	keys := []model.APIPubKey{}

	err := util.ReadJSONInto(resp.Body, &keys)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing keys list")
	}

	return keys, nil
}

func (c *communicatorImpl) AddPublicKey(ctx context.Context, keyName, keyValue string) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "keys",
	}

	key := model.APIPubKey{
		Name: model.APIString(keyName),
		Key:  model.APIString(keyValue),
	}

	resp, err := c.request(ctx, info, key)
	if err != nil {
		return errors.Wrap(err, "problem reaching evergreen API server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}

		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem adding key and parsing error message")
		}
		return errors.Wrap(errMsg, "problem adding key")
	}

	return nil
}

func (c *communicatorImpl) DeletePublicKey(ctx context.Context, keyName string) error {
	info := requestInfo{
		method:  delete,
		version: apiVersion2,
		path:    "keys/" + keyName,
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrap(err, "problem reaching evergreen API server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}

		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem deleting key and parsing error message")
		}
		return errors.Wrap(errMsg, "problem deleting key")
	}

	return nil
}
