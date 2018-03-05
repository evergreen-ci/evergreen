package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
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
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/terminate", hostID),
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, "")
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

func (c *communicatorImpl) ChangeSpawnHostPassword(ctx context.Context, hostID, rdpPassword string) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/change_password", hostID),
		version: apiVersion2,
	}
	body := model.APISpawnHostModify{
		RDPPwd: model.APIString(rdpPassword),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to change host RDP password")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem changing host RDP password and parsing error message")
		}
		return errors.Wrap(errMsg, "problem changing host RDP password")
	}
	return nil
}

func (c *communicatorImpl) ExtendSpawnHostExpiration(ctx context.Context, hostID string, addHours int) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/extend_expiration", hostID),
		version: apiVersion2,
	}
	body := model.APISpawnHostModify{
		AddHours: model.APIString(fmt.Sprintf("%d", addHours)),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to extend host expiration")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := rest.APIError{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem changing host expiration and parsing error message")
		}
		return errors.Wrap(errMsg, "problem changing host expiration")
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

func (c *communicatorImpl) SetBannerMessage(ctx context.Context, message string, theme evergreen.BannerTheme) error {
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
		path:    "admin/banner",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "problem getting current banner message")
	}

	banner := model.APIBanner{}
	if err = util.ReadJSONInto(resp.Body, &banner); err != nil {
		return "", errors.Wrap(err, "problem parsing response from server")
	}

	return string(banner.Text), nil
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

	return settings.ServiceFlags, nil
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

func (c *communicatorImpl) GetSettings(ctx context.Context) (*evergreen.Settings, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    "admin",
	}

	resp, client_err := c.request(ctx, info, "")
	if client_err != nil {
		return nil, errors.Wrap(client_err, "error retrieving settings")
	}
	defer resp.Body.Close()

	settings := &evergreen.Settings{}

	err := util.ReadJSONInto(resp.Body, settings)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing evergreen settings")
	}
	return settings, nil
}

func (c *communicatorImpl) UpdateSettings(ctx context.Context, update *model.APIAdminSettings) (*model.APIAdminSettings, error) {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin",
	}
	resp, err := c.request(ctx, info, &update)
	if err != nil {
		return nil, errors.Wrap(err, "error updating settings")
	}
	defer resp.Body.Close()

	newSettings := &model.APIAdminSettings{}
	err = util.ReadJSONInto(resp.Body, newSettings)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing evergreen settings")
	}

	return newSettings, nil
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

func (c *communicatorImpl) ListAliases(ctx context.Context, project string) ([]serviceModel.ProjectAlias, error) {
	path := fmt.Sprintf("alias/%s", project)
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem querying api server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("bad status from api server: %v", resp.StatusCode)
	}
	patchAliases := []serviceModel.ProjectAlias{}

	// use io.ReadAll and json.Unmarshal instead of util.ReadJSONInto since we may read the results twice
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading JSON")
	}
	if err := json.Unmarshal(bytes, &patchAliases); err != nil {
		patchAlias := serviceModel.ProjectAlias{}
		if err := json.Unmarshal(bytes, &patchAlias); err != nil {
			return nil, errors.Wrap(err, "error reading json")
		}
		patchAliases = []serviceModel.ProjectAlias{patchAlias}
	}
	return patchAliases, nil
}

func (c *communicatorImpl) GetClientConfig(ctx context.Context) (*evergreen.ClientConfig, error) {
	info := requestInfo{
		path:    "/status/cli_version",
		method:  get,
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch update manifest from server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("expected 200 OK from server, got %s", http.StatusText(resp.StatusCode))
	}
	update := &model.APICLIUpdate{}
	if err = util.ReadJSONInto(resp.Body, update); err != nil {
		return nil, errors.Wrap(err, "failed to parse update manifest from server")
	}

	configInterface, err := update.ClientConfig.ToService()
	if err != nil {
		return nil, err
	}
	config, ok := configInterface.(evergreen.ClientConfig)
	if !ok {
		return nil, errors.New("received client configuration is invalid")
	}
	if update.IgnoreUpdate {
		config.LatestRevision = evergreen.ClientVersion
	}

	return &config, nil
}
