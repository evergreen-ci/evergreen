package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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
func (c *communicatorImpl) CreateSpawnHost(ctx context.Context, spawnRequest *model.HostRequestOptions) (*model.APIHost, error) {

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

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return nil, errors.Wrap(err, "problem spawning host and parsing error message")
		}
		return nil, errors.Wrap(errMsg, "problem spawning host")
	}

	spawnHostResp := model.APIHost{}
	if err = util.ReadJSONInto(resp.Body, &spawnHostResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &spawnHostResp, nil
}

// ModifySpawnHost will start a job that updates the specified user-spawned host
// with the modifications passed as a parameter.
func (c *communicatorImpl) ModifySpawnHost(ctx context.Context, hostID string, changes host.HostModifyOptions) error {
	info := requestInfo{
		method:  patch,
		path:    fmt.Sprintf("hosts/%s", hostID),
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, changes)
	if err != nil {
		return errors.Wrap(err, "error sending request to modify host")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem modifying host and parsing error message")
		}
		return errors.Wrap(errMsg, "problem modifying host")
	}

	return nil
}

func (c *communicatorImpl) StopSpawnHost(ctx context.Context, hostID string, wait bool) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/stop", hostID),
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "error sending request to stop host")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem stopping host and parsing error message")
		}
		return errors.Wrap(errMsg, "problem stopping host")
	}

	if wait {
		return errors.Wrap(c.waitForStatus(ctx, hostID, evergreen.HostStopped), "problem waiting for host stop to complete")
	}

	return nil
}

func (c *communicatorImpl) AttachVolume(ctx context.Context, hostID string, opts *host.VolumeAttachment) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/attach", hostID),
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, opts)
	if err != nil {
		return errors.Wrap(err, "error sending request to attach volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem attaching volume and parsing error message")
		}
		return errors.Wrap(errMsg, "problem attaching volume")
	}

	return nil
}

func (c *communicatorImpl) DetachVolume(ctx context.Context, hostID, volumeID string) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/detach", hostID),
		version: apiVersion2,
	}
	body := host.VolumeAttachment{
		VolumeID: volumeID,
	}

	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrap(err, "error sending request to detach volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem detaching volume and parsing error message")
		}
		return errors.Wrap(errMsg, "problem detaching volume")
	}

	return nil
}
func (c *communicatorImpl) CreateVolume(ctx context.Context, volume *host.Volume) (*model.APIVolume, error) {
	info := requestInfo{
		method:  post,
		path:    "volumes",
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, volume)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to create volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("The status: %d\n", resp.StatusCode)
		respErr, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("%v", string(respErr))
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return nil, errors.Wrap(err, "problem creating volume and parsing error message")
		}
		return nil, errors.Wrap(errMsg, "problem creating volume")
	}

	createVolumeResp := model.APIVolume{}
	if err = util.ReadJSONInto(resp.Body, &createVolumeResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &createVolumeResp, nil
}

func (c *communicatorImpl) DeleteVolume(ctx context.Context, volumeID string) error {
	info := requestInfo{
		method:  delete,
		path:    fmt.Sprintf("volumes/%s", volumeID),
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrap(err, "error sending request to delete volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem deleting volume and parsing error message")
		}
		return errors.Wrap(errMsg, "problem deleting volume")
	}

	return nil
}

func (c *communicatorImpl) StartSpawnHost(ctx context.Context, hostID string, wait bool) error {
	info := requestInfo{
		method:  post,
		path:    fmt.Sprintf("hosts/%s/start", hostID),
		version: apiVersion2,
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "error sending request to start host")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrap(err, "problem starting host and parsing error message")
		}
		return errors.Wrap(errMsg, "problem starting host")
	}

	if wait {
		return errors.Wrap(c.waitForStatus(ctx, hostID, evergreen.HostRunning), "problem waiting for host start to complete")
	}

	return nil
}

func (c *communicatorImpl) waitForStatus(ctx context.Context, hostID, status string) error {
	const (
		contextTimeout = 10 * time.Minute
		retryInterval  = 10 * time.Second
	)

	info := requestInfo{
		method:  get,
		path:    fmt.Sprintf("hosts/%s", hostID),
		version: apiVersion2,
	}

	timerCtx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()
	timer := time.NewTimer(0)
	for {
		select {
		case <-timerCtx.Done():
			return errors.New("timer context canceled")
		case <-timer.C:
			resp, err := c.request(ctx, info, "")
			if err != nil {
				return errors.Wrap(err, "error sending request to get host info")
			}
			if resp.StatusCode != http.StatusOK {
				errMsg := gimlet.ErrorResponse{}
				if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
					return errors.Wrap(err, "problem getting host and parsing error message")
				}
				return errors.Wrap(errMsg, "problem getting host")
			}
			hostResp := model.APIHost{}
			if err = util.ReadJSONInto(resp.Body, &hostResp); err != nil {
				return fmt.Errorf("Error forming response body response: %v", err)
			}
			if model.FromAPIString(hostResp.Status) == status {
				return nil
			}
			timer.Reset(retryInterval)
		}
	}
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
		errMsg := gimlet.ErrorResponse{}
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
		RDPPwd: model.ToAPIString(rdpPassword),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to change host RDP password")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
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
		AddHours: model.ToAPIString(fmt.Sprintf("%d", addHours)),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to extend host expiration")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
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

	return model.FromAPIString(banner.Text), nil
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
		path:    "admin/settings",
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

func (c *communicatorImpl) GetEvents(ctx context.Context, ts time.Time, limit int) ([]interface{}, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    fmt.Sprintf("admin/events?ts=%s&limit=%d", ts.Format(time.RFC3339), limit),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error updating settings")
	}
	defer resp.Body.Close()

	events := []interface{}{}
	err = util.ReadJSONInto(resp.Body, &events)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing response")
	}

	return events, nil
}

func (c *communicatorImpl) RevertSettings(ctx context.Context, guid string) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin/revert",
	}
	body := struct {
		GUID string `json:"guid"`
	}{guid}
	resp, err := c.request(ctx, info, &body)
	if err != nil {
		return errors.Wrap(err, "error reverting settings")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error reverting %s", guid)
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
		errMsg := gimlet.ErrorResponse{}

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
		Name: model.ToAPIString(keyName),
		Key:  model.ToAPIString(keyValue),
	}

	resp, err := c.request(ctx, info, key)
	if err != nil {
		return errors.Wrap(err, "problem reaching evergreen API server")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}

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
		errMsg := gimlet.ErrorResponse{}

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

func (c *communicatorImpl) GetSubscriptions(ctx context.Context) ([]event.Subscription, error) {
	info := requestInfo{
		path:    fmt.Sprintf("/subscriptions?owner=%s&type=person", c.apiUser),
		method:  get,
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}
	if resp.StatusCode != http.StatusOK {
		var restErr gimlet.ErrorResponse
		if err = json.Unmarshal(bytes, &restErr); err != nil {
			return nil, errors.Errorf("expected 200 OK while fetching subscriptions, got %s. Raw response was: %s", resp.Status, string(bytes))
		}

		restErr = gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Unknown error",
		}

		return nil, errors.Wrap(restErr, "server returned error while fetching subscriptions")
	}

	apiSubs := []model.APISubscription{}

	if err = json.Unmarshal(bytes, &apiSubs); err != nil {
		apiSub := model.APISubscription{}
		if err = json.Unmarshal(bytes, &apiSub); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal subscriptions")
		}

		apiSubs = append(apiSubs, apiSub)
	}

	subs := make([]event.Subscription, len(apiSubs))
	for i := range apiSubs {
		var iface interface{}
		iface, err = apiSubs[i].ToService()
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert api model")
		}

		var ok bool
		subs[i], ok = iface.(event.Subscription)
		if !ok {
			return nil, errors.New("received unexpected type from server")
		}
	}

	return subs, nil
}

func (c *communicatorImpl) CreateVersionFromConfig(ctx context.Context, project, message string, active bool, config []byte) (*serviceModel.Version, error) {
	info := requestInfo{
		method:  put,
		version: apiVersion2,
		path:    "/versions",
	}
	body := struct {
		ProjectID string          `json:"project_id"`
		Message   string          `json:"message"`
		Active    bool            `json:"activate"`
		Config    json.RawMessage `json:"config"`
	}{
		ProjectID: project,
		Message:   message,
		Active:    active,
		Config:    config,
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}
	if resp.StatusCode != http.StatusOK {
		restErr := gimlet.ErrorResponse{}
		if err = json.Unmarshal(bytes, &restErr); err != nil {
			return nil, errors.Errorf("received an error but was unable to parse: %s", string(bytes))
		}

		return nil, errors.Wrap(restErr, "error while creating version from config")
	}
	v := &serviceModel.Version{}
	err = json.Unmarshal(bytes, v)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing version data")
	}

	return v, nil
}

func (c *communicatorImpl) GetCommitQueue(ctx context.Context, projectID string) (*model.APICommitQueue, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    fmt.Sprintf("/commit_queue/%s", projectID),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem fetching commit queue list")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}

		if err = util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return nil, errors.Wrap(err, "problem fetching commit queue list and parsing error message")
		}
		return nil, errMsg
	}

	cq := model.APICommitQueue{}

	if err = util.ReadJSONInto(resp.Body, &cq); err != nil {
		return nil, errors.Wrap(err, "error parsing commit queue")
	}

	return &cq, nil
}

func (c *communicatorImpl) DeleteCommitQueueItem(ctx context.Context, projectID, item string) error {
	info := requestInfo{
		method:  delete,
		version: apiVersion2,
		path:    fmt.Sprintf("/commit_queue/%s/%s", projectID, item),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "problem deleting item '%s' from commit queue '%s'", item, projectID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrapf(err, "response code %d problem deleting item '%s' from commit queue '%s' and parsing error message", resp.StatusCode, item, projectID)
		}
		return errMsg
	}

	return nil
}

func (c *communicatorImpl) EnqueueItem(ctx context.Context, patchID string) (int, error) {
	info := requestInfo{
		method:  put,
		version: apiVersion2,
		path:    fmt.Sprintf("/commit_queue/%s", patchID),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read response")
	}
	if resp.StatusCode != http.StatusOK {
		restErr := gimlet.ErrorResponse{}
		if err = json.Unmarshal(bytes, &restErr); err != nil {
			return 0, errors.Errorf("received an error but was unable to parse: %s", string(bytes))
		}

		return 0, restErr
	}

	positionResp := model.APICommitQueuePosition{}
	if err = json.Unmarshal(bytes, &positionResp); err != nil {
		return 0, errors.Wrap(err, "error parsing position response")
	}

	return positionResp.Position, nil
}

func (c *communicatorImpl) GetCommitQueueItemAuthor(ctx context.Context, projectID, item string) (string, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    fmt.Sprintf("/commit_queue/%s/%s/author", projectID, item),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "error making request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err = util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return "", errors.Wrap(err, "problem getting author and parsing error message")
		}
		return "", errors.Wrap(errMsg, "problem getting author")
	}
	authorResp := model.APICommitQueueItemAuthor{}
	if err = util.ReadJSONInto(resp.Body, &authorResp); err != nil {
		return "", errors.Wrap(err, "error parsing author response")
	}
	return model.FromAPIString(authorResp.Author), nil
}

func (c *communicatorImpl) SendNotification(ctx context.Context, notificationType string, data interface{}) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "notifications/" + notificationType,
	}

	resp, err := c.request(ctx, info, data)
	if err != nil {
		return errors.Wrapf(err, "problem sending slack notification")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return errors.Wrapf(err, "response code %d problem sending '%s' notification and parsing errors message",
				resp.StatusCode, notificationType)
		}
		return errMsg
	}

	return nil
}

// GetDockerStatus returns status of the container for the given host
func (c *communicatorImpl) GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error) {
	info := requestInfo{
		method:  get,
		path:    fmt.Sprintf("hosts/%s/status", hostID),
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting container status for %s", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := gimlet.ErrorResponse{}
		if err := util.ReadJSONInto(resp.Body, &errMsg); err != nil {
			return nil, errors.Wrap(err, "problem getting container status and parsing error message")
		}
		return nil, errors.Wrap(errMsg, "problem getting container status")
	}
	status := cloud.ContainerStatus{}
	if err := util.ReadJSONInto(resp.Body, &status); err != nil {
		return nil, errors.Wrap(err, "problem parsing container status")
	}

	return &status, nil
}

func (c *communicatorImpl) GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error) {
	path := fmt.Sprintf("/hosts/%s/logs", hostID)
	if isError {
		path = fmt.Sprintf("%s/error", path)
	} else {
		path = fmt.Sprintf("%s/output", path)
	}
	if !util.IsZeroTime(startTime) && !util.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?start_time=%s&end_time=%s", path, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	} else if !util.IsZeroTime(startTime) {
		path = fmt.Sprintf("%s?start_time=%s", path, startTime.Format(time.RFC3339))
	} else if !util.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?end_time=%s", path, endTime.Format(time.RFC3339))
	}

	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting logs for container _id %s", hostID)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	if resp.StatusCode != http.StatusOK {
		restErr := gimlet.ErrorResponse{}
		if err = json.Unmarshal(body, &restErr); err != nil {
			return nil, errors.Errorf("received an error but was unable to parse: %s", string(body))
		}

		return nil, errors.Wrapf(restErr, "response code %d problem getting logs for container _id %s",
			resp.StatusCode, hostID)
	}
	return body, nil
}

func (c *communicatorImpl) GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    fmt.Sprintf("/tasks/%s/manifest", taskId),
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting manifest for task '%s'", taskId)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		restErr := gimlet.ErrorResponse{}
		if err = util.ReadJSONInto(resp.Body, &restErr); err != nil {
			return nil, errors.Wrap(err, "received an error but was unable to parse")
		}
		return nil, errors.Wrapf(restErr, "response code %d problem getting manifest for task '%s'",
			resp.StatusCode, taskId)
	}
	mfest := manifest.Manifest{}
	if err := util.ReadJSONInto(resp.Body, &mfest); err != nil {
		return nil, errors.Wrap(err, "problem parsing manifest")
	}

	return &mfest, nil
}
