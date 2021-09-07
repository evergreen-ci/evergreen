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
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// CreateSpawnHost will insert an intent host into the DB that will be spawned later by the runner
func (c *communicatorImpl) CreateSpawnHost(ctx context.Context, spawnRequest *model.HostRequestOptions) (*model.APIHost, error) {

	info := requestInfo{
		method: http.MethodPost,
		path:   "hosts",
	}
	resp, err := c.request(ctx, info, spawnRequest)
	if err != nil {
		err = errors.Wrapf(err, "error sending request to spawn host")
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "spawning host")
	}

	spawnHostResp := model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &spawnHostResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &spawnHostResp, nil
}

// GetSpawnHost will return the host document for the given hostId
func (c *communicatorImpl) GetSpawnHost(ctx context.Context, hostId string) (*model.APIHost, error) {

	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("hosts/%s", hostId),
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		err = errors.Wrapf(err, "error sending request to spawn host")
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting host '%s'", hostId)
	}

	spawnHostResp := model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &spawnHostResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &spawnHostResp, nil
}

// ModifySpawnHost will start a job that updates the specified user-spawned host
// with the modifications passed as a parameter.
func (c *communicatorImpl) ModifySpawnHost(ctx context.Context, hostID string, changes host.HostModifyOptions) error {
	info := requestInfo{
		method: http.MethodPatch,
		path:   fmt.Sprintf("hosts/%s", hostID),
	}

	resp, err := c.request(ctx, info, changes)
	if err != nil {
		return errors.Wrap(err, "error sending request to modify host")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "modifying host '%s'", hostID)
	}

	return nil
}

func (c *communicatorImpl) StopSpawnHost(ctx context.Context, hostID string, subscriptionType string, wait bool) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/stop", hostID),
	}

	options := struct {
		SubscriptionType string `json:"subscription_type"`
	}{SubscriptionType: subscriptionType}

	resp, err := c.request(ctx, info, options)
	if err != nil {
		return errors.Wrapf(err, "error sending request to stop host")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "stopping host '%s'", hostID)
	}

	if wait {
		return errors.Wrap(c.waitForStatus(ctx, hostID, evergreen.HostStopped), "problem waiting for host stop to complete")
	}

	return nil
}

func (c *communicatorImpl) AttachVolume(ctx context.Context, hostID string, opts *host.VolumeAttachment) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/attach", hostID),
	}

	resp, err := c.request(ctx, info, opts)
	if err != nil {
		return errors.Wrap(err, "error sending request to attach volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "attaching volume to host '%s'", hostID)
	}

	return nil
}

func (c *communicatorImpl) DetachVolume(ctx context.Context, hostID, volumeID string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/detach", hostID),
	}
	body := host.VolumeAttachment{
		VolumeID: volumeID,
	}

	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrap(err, "error sending request to detach volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "detaching volume '%s' from host '%s'", volumeID, hostID)
	}

	return nil
}

func (c *communicatorImpl) CreateVolume(ctx context.Context, volume *host.Volume) (*model.APIVolume, error) {
	info := requestInfo{
		method: http.MethodPost,
		path:   "volumes",
	}

	resp, err := c.request(ctx, info, volume)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to create volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "creating volume")
	}

	createVolumeResp := model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, &createVolumeResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &createVolumeResp, nil
}

func (c *communicatorImpl) DeleteVolume(ctx context.Context, volumeID string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   fmt.Sprintf("volumes/%s", volumeID),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrap(err, "error sending request to delete volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "deleting volume '%s'", volumeID)
	}

	return nil
}

func (c *communicatorImpl) ModifyVolume(ctx context.Context, volumeID string, opts *model.VolumeModifyOptions) error {
	info := requestInfo{
		method: http.MethodPatch,
		path:   fmt.Sprintf("volumes/%s", volumeID),
	}
	resp, err := c.request(ctx, info, opts)
	if err != nil {
		return errors.Wrap(err, "error sending request to modify volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "modifying volume '%s'", volumeID)
	}

	return nil
}

func (c *communicatorImpl) GetVolume(ctx context.Context, volumeID string) (*model.APIVolume, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("volumes/%s", volumeID),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to get volumes")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting volume '%s'", volumeID)
	}

	volumeResp := &model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, volumeResp); err != nil {
		return nil, fmt.Errorf("error forming response body response: %v", err)
	}

	return volumeResp, nil
}

func (c *communicatorImpl) GetVolumesByUser(ctx context.Context) ([]model.APIVolume, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "volumes",
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to get volumes")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting volumes for user '%s'", c.apiUser)
	}

	getVolumesResp := []model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, &getVolumesResp); err != nil {
		return nil, fmt.Errorf("error forming response body response: %v", err)
	}

	return getVolumesResp, nil
}

func (c *communicatorImpl) StartSpawnHost(ctx context.Context, hostID string, subscriptionType string, wait bool) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/start", hostID),
	}

	options := struct {
		SubscriptionType string `json:"subscription_type"`
	}{SubscriptionType: subscriptionType}

	resp, err := c.request(ctx, info, options)
	if err != nil {
		return errors.Wrapf(err, "error sending request to start host")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "starting host '%s'", hostID)
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
		method: http.MethodGet,
		path:   fmt.Sprintf("hosts/%s", hostID),
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
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				return AuthError
			}
			if resp.StatusCode != http.StatusOK {
				return utility.RespErrorf(resp, "getting host status")
			}
			hostResp := model.APIHost{}
			if err = utility.ReadJSON(resp.Body, &hostResp); err != nil {
				return fmt.Errorf("Error forming response body response: %v", err)
			}
			if utility.FromStringPtr(hostResp.Status) == status {
				return nil
			}
			timer.Reset(retryInterval)
		}
	}
}

func (c *communicatorImpl) TerminateSpawnHost(ctx context.Context, hostID string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/terminate", hostID),
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "error sending request to terminate host")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "terminating host '%s'", hostID)
	}

	return nil
}

func (c *communicatorImpl) ChangeSpawnHostPassword(ctx context.Context, hostID, rdpPassword string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/change_password", hostID),
	}
	body := model.APISpawnHostModify{
		RDPPwd: utility.ToStringPtr(rdpPassword),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to change host RDP password")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "changing RDP password on host '%s'", hostID)
	}

	return nil
}

func (c *communicatorImpl) ExtendSpawnHostExpiration(ctx context.Context, hostID string, addHours int) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/extend_expiration", hostID),
	}
	body := model.APISpawnHostModify{
		AddHours: utility.ToStringPtr(fmt.Sprintf("%d", addHours)),
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error sending request to extend host expiration")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "changing expiration of host '%s'", hostID)
	}
	return nil
}

// GetHosts gets all hosts matching filters
func (c *communicatorImpl) GetHosts(ctx context.Context, data model.APIHostParams) ([]*model.APIHost, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "host/filter",
	}

	resp, err := c.request(ctx, info, data)
	if err != nil {
		err = errors.Wrapf(err, "error sending request to spawn host")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting hosts")
	}

	hosts := []*model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &hosts); err != nil {
		return nil, errors.Wrap(err, "can't read response as APIHost slice")
	}
	return hosts, nil
}

func (c *communicatorImpl) SetBannerMessage(ctx context.Context, message string, theme evergreen.BannerTheme) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/banner",
	}

	resp, err := c.retryRequest(ctx, info, struct {
		Banner string `json:"banner"`
		Theme  string `json:"theme"`
	}{
		Banner: message,
		Theme:  string(theme),
	})
	if err != nil {
		return utility.RespErrorf(resp, "failed to set banner: %s", err.Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetBannerMessage(ctx context.Context) (string, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "admin/banner",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "problem getting current banner message")
	}
	defer resp.Body.Close()

	banner := model.APIBanner{}
	if err = utility.ReadJSON(resp.Body, &banner); err != nil {
		return "", errors.Wrap(err, "problem parsing response from server")
	}

	return utility.FromStringPtr(banner.Text), nil
}

func (c *communicatorImpl) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/service_flags",
	}

	resp, err := c.retryRequest(ctx, info, f)
	if err != nil {
		return utility.RespErrorf(resp, "failed to set service flags: %s", err.Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "admin",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting service flags")
	}
	defer resp.Body.Close()

	settings := model.APIAdminSettings{}
	if err = utility.ReadJSON(resp.Body, &settings); err != nil {
		return nil, errors.Wrap(err, "problem parsing service flag response")
	}

	return settings.ServiceFlags, nil
}

func (c *communicatorImpl) RestartRecentTasks(ctx context.Context, startAt, endAt time.Time) error {
	if endAt.Before(startAt) {
		return errors.Errorf("start (%s) cannot be before end (%s)", startAt, endAt)
	}

	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/restart",
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

	resp, err := c.request(ctx, info, &payload)
	if err != nil {
		return errors.Wrap(err, "problem restarting recent tasks")
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetSettings(ctx context.Context) (*evergreen.Settings, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "admin/settings",
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving settings")
	}
	defer resp.Body.Close()

	settings := &evergreen.Settings{}

	if err = utility.ReadJSON(resp.Body, settings); err != nil {
		return nil, errors.Wrap(err, "error parsing evergreen settings")
	}
	return settings, nil
}

func (c *communicatorImpl) UpdateSettings(ctx context.Context, update *model.APIAdminSettings) (*model.APIAdminSettings, error) {
	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/settings",
	}
	resp, err := c.request(ctx, info, &update)
	if err != nil {
		return nil, errors.Wrap(err, "error updating settings")
	}
	defer resp.Body.Close()

	newSettings := &model.APIAdminSettings{}
	err = utility.ReadJSON(resp.Body, newSettings)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing evergreen settings")
	}

	return newSettings, nil
}

func (c *communicatorImpl) GetEvents(ctx context.Context, ts time.Time, limit int) ([]interface{}, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("admin/events?ts=%s&limit=%d", ts.Format(time.RFC3339), limit),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error updating settings")
	}
	defer resp.Body.Close()

	events := []interface{}{}
	err = utility.ReadJSON(resp.Body, &events)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing response")
	}

	return events, nil
}

func (c *communicatorImpl) RevertSettings(ctx context.Context, guid string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/revert",
	}
	body := struct {
		GUID string `json:"guid"`
	}{guid}
	resp, err := c.request(ctx, info, &body)
	if err != nil {
		return errors.Wrap(err, "error reverting settings")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error reverting %s", guid)
	}

	return nil
}

func (c *communicatorImpl) ExecuteOnDistro(ctx context.Context, distro string, opts model.APIDistroScriptOptions) (hostIDs []string, err error) {
	info := requestInfo{
		method: http.MethodPatch,
		path:   fmt.Sprintf("/distros/%s/execute", distro),
	}

	var result struct {
		HostIDs []string `json:"host_ids"`
	}
	resp, err := c.request(ctx, info, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem during request")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "running script on distro '%s'", distro)
	}

	if err = utility.ReadJSON(resp.Body, &result); err != nil {
		return nil, errors.Wrap(err, "problem reading response")
	}
	return result.HostIDs, nil
}

func (c *communicatorImpl) GetServiceUsers(ctx context.Context) ([]model.APIDBUser, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "/admin/service_users",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem during request")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting service users")
	}
	var result []model.APIDBUser
	if err = utility.ReadJSON(resp.Body, &result); err != nil {
		return nil, errors.Wrap(err, "problem reading response")
	}

	return result, nil
}

func (c *communicatorImpl) UpdateServiceUser(ctx context.Context, username, displayName string, roles []string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "/admin/service_users",
	}
	body := model.APIDBUser{
		UserID:      &username,
		DisplayName: &displayName,
		Roles:       roles,
	}

	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrap(err, "problem during request")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "updating service user")
	}

	return nil
}

func (c *communicatorImpl) DeleteServiceUser(ctx context.Context, username string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   fmt.Sprintf("/admin/service_users?id=%s", username),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return errors.Wrap(err, "problem during request")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "deleting service user")
	}

	return nil
}

func (c *communicatorImpl) GetDistrosList(ctx context.Context) ([]model.APIDistro, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "distros",
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem fetching distribution list")
	}
	defer resp.Body.Close()

	distros := []model.APIDistro{}

	if err = utility.ReadJSON(resp.Body, &distros); err != nil {
		return nil, errors.Wrap(err, "error parsing distribution list")
	}

	return distros, nil
}

func (c *communicatorImpl) GetCurrentUsersKeys(ctx context.Context) ([]model.APIPubKey, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "keys",
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem fetching keys list")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting key list")
	}

	keys := []model.APIPubKey{}

	if err = utility.ReadJSON(resp.Body, &keys); err != nil {
		return nil, errors.Wrap(err, "error parsing keys list")
	}

	return keys, nil
}

func (c *communicatorImpl) AddPublicKey(ctx context.Context, keyName, keyValue string) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "keys",
	}

	key := model.APIPubKey{
		Name: utility.ToStringPtr(keyName),
		Key:  utility.ToStringPtr(keyValue),
	}

	resp, err := c.request(ctx, info, key)
	if err != nil {
		return errors.Wrap(err, "problem reaching evergreen API server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "adding key")
	}

	return nil
}

func (c *communicatorImpl) DeletePublicKey(ctx context.Context, keyName string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   "keys/" + keyName,
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrap(err, "problem reaching evergreen API server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "deleting key")
	}

	return nil
}

func (c *communicatorImpl) ListAliases(ctx context.Context, project string) ([]serviceModel.ProjectAlias, error) {
	path := fmt.Sprintf("alias/%s", project)
	info := requestInfo{
		method: http.MethodGet,
		path:   path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem querying api server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("bad status from api server: %v", resp.StatusCode)
	}
	patchAliases := []serviceModel.ProjectAlias{}

	// use io.ReadAll and json.Unmarshal instead of utility.ReadJSON since we may read the results twice
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

func (c *communicatorImpl) ListPatchTriggerAliases(ctx context.Context, project string) ([]string, error) {
	path := fmt.Sprintf("projects/%s/patch_trigger_aliases", project)
	info := requestInfo{
		method: http.MethodGet,
		path:   path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem querying api server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("bad status from api server: %v", resp.StatusCode)
	}

	triggerAliases := []string{}
	if err = utility.ReadJSON(resp.Body, &triggerAliases); err != nil {
		return nil, errors.Wrap(err, "failed to parse update manifest from server")
	}

	return triggerAliases, nil
}

func (c *communicatorImpl) GetClientConfig(ctx context.Context) (*evergreen.ClientConfig, error) {
	info := requestInfo{
		path:   "/status/cli_version",
		method: http.MethodGet,
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch update manifest from server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("expected 200 OK from server, got %s", http.StatusText(resp.StatusCode))
	}
	update := &model.APICLIUpdate{}
	if err = utility.ReadJSON(resp.Body, update); err != nil {
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

func (c *communicatorImpl) GetParameters(ctx context.Context, project string) ([]serviceModel.ParameterInfo, error) {
	path := fmt.Sprintf("projects/%s/parameters", project)
	info := requestInfo{
		method: http.MethodGet,
		path:   path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem querying api server")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		grip.Error(resp.Body)
		return nil, errors.Errorf("bad status from api server: %v", resp.StatusCode)
	}

	params := []serviceModel.ParameterInfo{}
	if err = utility.ReadJSON(resp.Body, &params); err != nil {
		return nil, errors.Wrap(err, "error parsing parameters")
	}
	return params, nil
}

func (c *communicatorImpl) GetSubscriptions(ctx context.Context) ([]event.Subscription, error) {
	info := requestInfo{
		path:   fmt.Sprintf("/subscriptions?owner=%s&type=person", c.apiUser),
		method: http.MethodGet,
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscriptions")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting subscriptions")
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
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
		method: http.MethodPut,
		path:   "/versions",
	}
	body := struct {
		ProjectID string          `json:"project_id"`
		Message   string          `json:"message"`
		Active    bool            `json:"activate"`
		IsAdHoc   bool            `json:"is_adhoc"`
		Config    json.RawMessage `json:"config"`
	}{
		ProjectID: project,
		Message:   message,
		Active:    active,
		IsAdHoc:   true,
		Config:    config,
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "creating version from config")
	}

	v := &serviceModel.Version{}
	if err = utility.ReadJSON(resp.Body, v); err != nil {
		return nil, errors.Wrap(err, "parsing version data")
	}

	return v, nil
}

func (c *communicatorImpl) GetCommitQueue(ctx context.Context, projectID string) (*model.APICommitQueue, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/commit_queue/%s", projectID),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "problem fetching commit queue list")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "fetching commit queue list")
	}

	cq := model.APICommitQueue{}
	if err = utility.ReadJSON(resp.Body, &cq); err != nil {
		return nil, errors.Wrap(err, "error parsing commit queue")
	}

	return &cq, nil
}

func (c *communicatorImpl) DeleteCommitQueueItem(ctx context.Context, projectID, item string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   fmt.Sprintf("/commit_queue/%s/%s", projectID, item),
	}

	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "problem deleting item '%s' from commit queue '%s'", item, projectID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return utility.RespErrorf(resp, "problem deleting item '%s' from commit queue '%s'", item, projectID)
	}

	return nil
}

func (c *communicatorImpl) EnqueueItem(ctx context.Context, patchID string, enqueueNext bool) (int, error) {
	info := requestInfo{
		method: http.MethodPut,
		path:   fmt.Sprintf("/commit_queue/%s", patchID),
	}
	if enqueueNext {
		info.path += "?force=true"
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return 0, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return 0, utility.RespErrorf(resp, "enqueueing commit queue item")
	}

	positionResp := model.APICommitQueuePosition{}
	if err = utility.ReadJSON(resp.Body, &positionResp); err != nil {
		return 0, errors.Wrap(err, "parsing position response")
	}

	return positionResp.Position, nil
}

func (c *communicatorImpl) CreatePatchForMerge(ctx context.Context, patchID, commitMessage string) (*model.APIPatch, error) {
	info := requestInfo{
		method: http.MethodPut,
		path:   fmt.Sprintf("/patches/%s/merge_patch", patchID),
	}
	body := struct {
		CommitMessage string `json:"commit_message"`
	}{CommitMessage: commitMessage}

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
			return nil, errors.Errorf("received status code '%d' but was unable to parse error: '%s'", resp.StatusCode, string(bytes))
		}
		return nil, restErr
	}

	newPatch := &model.APIPatch{}
	if err = json.Unmarshal(bytes, newPatch); err != nil {
		return nil, errors.Wrap(err, "error parsing position response")
	}

	return newPatch, nil
}

func (c *communicatorImpl) GetMessageForPatch(ctx context.Context, patchID string) (string, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/commit_queue/%s/message", patchID),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response")
	}
	if resp.StatusCode != http.StatusOK {
		restErr := gimlet.ErrorResponse{}
		if err = json.Unmarshal(bytes, &restErr); err != nil {
			return "", errors.Errorf("received status code '%d' but was unable to parse error: '%s'", resp.StatusCode, string(bytes))
		}
		return "", restErr
	}

	var message string
	if err = json.Unmarshal(bytes, &message); err != nil {
		return "", errors.Wrap(err, "error parsing position response")
	}

	return message, nil
}

func (c *communicatorImpl) SendNotification(ctx context.Context, notificationType string, data interface{}) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "notifications/" + notificationType,
	}

	resp, err := c.request(ctx, info, data)
	if err != nil {
		return errors.Wrapf(err, "problem sending slack notification")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "sending '%s' notification", notificationType)
	}

	return nil
}

// GetDockerStatus returns status of the container for the given host
func (c *communicatorImpl) GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("hosts/%s/status", hostID),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting container status for %s", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting container status")
	}
	status := cloud.ContainerStatus{}
	if err := utility.ReadJSON(resp.Body, &status); err != nil {
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
	if !utility.IsZeroTime(startTime) && !utility.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?start_time=%s&end_time=%s", path, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	} else if !utility.IsZeroTime(startTime) {
		path = fmt.Sprintf("%s?start_time=%s", path, startTime.Format(time.RFC3339))
	} else if !utility.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?end_time=%s", path, endTime.Format(time.RFC3339))
	}

	info := requestInfo{
		method: http.MethodGet,
		path:   path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting logs for container _id %s", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting logs for container id '%s'", hostID)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	return body, nil
}

func (c *communicatorImpl) GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/tasks/%s/manifest", taskId),
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting manifest for task '%s'", taskId)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting manifest for task '%s'", taskId)
	}
	mfest := manifest.Manifest{}
	if err := utility.ReadJSON(resp.Body, &mfest); err != nil {
		return nil, errors.Wrap(err, "problem parsing manifest")
	}

	return &mfest, nil
}

func (c *communicatorImpl) StartHostProcesses(ctx context.Context, hostIDs []string, script string, batchSize int) ([]model.APIHostProcess, error) {
	info := requestInfo{
		method: http.MethodPost,
		path:   "/host/start_processes",
	}

	result := []model.APIHostProcess{}
	for i := 0; i < len(hostIDs); i += batchSize {
		end := i + batchSize
		if end > len(hostIDs) {
			end = len(hostIDs)
		}
		data := model.APIHostScript{Hosts: hostIDs[i:end], Script: script}
		output, err := func() ([]model.APIHostProcess, error) {
			resp, err := c.request(ctx, info, data)
			if err != nil {
				return nil, errors.Wrap(err, "can't make request to run script on host")
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusUnauthorized {
				return nil, AuthError
			}
			if resp.StatusCode != http.StatusOK {
				return nil, utility.RespErrorf(resp, "running script on host")
			}

			output := []model.APIHostProcess{}
			if err := utility.ReadJSON(resp.Body, &output); err != nil {
				return nil, errors.Wrap(err, "problem reading response")
			}

			return output, nil
		}()
		if err != nil {
			return nil, errors.Wrap(err, "can't start processes")
		}

		result = append(result, output...)
	}

	return result, nil
}

func (c *communicatorImpl) GetHostProcessOutput(ctx context.Context, hostProcesses []model.APIHostProcess, batchSize int) ([]model.APIHostProcess, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "/host/get_processes",
	}

	result := []model.APIHostProcess{}

	for i := 0; i < len(hostProcesses); i += batchSize {
		end := i + batchSize
		if end > len(hostProcesses) {
			end = len(hostProcesses)
		}
		output, err := func() ([]model.APIHostProcess, error) {
			resp, err := c.request(ctx, info, hostProcesses[i:end])
			if err != nil {
				return nil, errors.Wrap(err, "can't make request to run script on host")
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusUnauthorized {
				return nil, AuthError
			}
			if resp.StatusCode != http.StatusOK {
				return nil, utility.RespErrorf(resp, "running script on host")
			}

			output := []model.APIHostProcess{}
			if err := utility.ReadJSON(resp.Body, &output); err != nil {
				return nil, errors.Wrap(err, "problem reading response")
			}

			return output, nil
		}()
		if err != nil {
			return nil, errors.Wrap(err, "can't get process output")
		}

		result = append(result, output...)
	}

	return result, nil
}

func (c *communicatorImpl) GetRecentVersionsForProject(ctx context.Context, projectID, requester string) ([]model.APIVersion, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("projects/%s/versions?requester=%s", projectID, requester),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to get versions")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "problem getting versions for project '%s'", projectID)
	}

	getVersionsResp := []model.APIVersion{}
	if err = utility.ReadJSON(resp.Body, &getVersionsResp); err != nil {
		return nil, fmt.Errorf("error forming response body response: %v", err)
	}

	return getVersionsResp, nil
}

func (c *communicatorImpl) GetTaskSyncReadCredentials(ctx context.Context) (*evergreen.S3Credentials, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "/task/sync_read_credentials",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't make request to get task read-only credentials")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting task read-only credentials")
	}
	creds := &evergreen.S3Credentials{}
	if err := utility.ReadJSON(resp.Body, creds); err != nil {
		return nil, errors.Wrap(err, "reading credentials from response body")
	}

	return creds, nil
}

func (c *communicatorImpl) GetTaskSyncPath(ctx context.Context, taskID string) (string, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/tasks/%s/sync_path", taskID),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "couldn't make request to get task read-only credentials")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return "", utility.RespErrorf(resp, "getting task sync path")
	}
	path, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading task sync path from response body")
	}

	return string(path), nil
}

func (c *communicatorImpl) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("distros/%s", id),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get distro named %s: %s", id, err.Error())
	}
	defer resp.Body.Close()

	d := &restmodel.APIDistro{}
	if err = utility.ReadJSON(resp.Body, &d); err != nil {
		return nil, errors.Wrapf(err, "reading distro from response body for '%s'", id)
	}

	return d, nil

}

func (c *communicatorImpl) GetClientURLs(ctx context.Context, distroID string) ([]string, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("distros/%s/client_urls", distroID),
	}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get clients: %s", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting client URLs")
	}

	var urls []string
	if err := utility.ReadJSON(resp.Body, &urls); err != nil {
		return nil, errors.Wrapf(err, "reading client URLs from response")
	}

	return urls, nil
}

func (c *communicatorImpl) GetHostProvisioningOptions(ctx context.Context, hostID, hostSecret string) (*restmodel.APIHostProvisioningOptions, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/hosts/%s/provisioning_options", hostID),
	}
	r, err := c.createRequest(info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}
	r.Header.Add(evergreen.HostHeader, hostID)
	r.Header.Add(evergreen.HostSecretHeader, hostSecret)
	resp, err := utility.RetryRequest(ctx, r, utility.RetryOptions{
		MaxAttempts: c.maxAttempts,
		MinDelay:    c.timeoutStart,
		MaxDelay:    c.timeoutMax,
	})
	if err != nil {
		return nil, utility.RespErrorf(resp, "making request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "received error response")
	}
	var opts restmodel.APIHostProvisioningOptions
	if err = utility.ReadJSON(resp.Body, &opts); err != nil {
		return nil, errors.Wrap(err, "reading response")
	}
	return &opts, nil
}

func (c *communicatorImpl) CompareTasks(ctx context.Context, tasks []string, useLegacy bool) ([]string, map[string]map[string]string, error) {
	info := requestInfo{
		method: http.MethodPost,
		path:   "/scheduler/compare_tasks",
	}
	body := restmodel.CompareTasksRequest{
		Tasks:     tasks,
		UseLegacy: useLegacy,
	}
	r, err := c.createRequest(info, body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not create request")
	}
	resp, err := c.doRequest(ctx, r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not make request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, utility.RespErrorf(resp, "received error response")
	}
	var results restmodel.CompareTasksResponse
	if err = utility.ReadJSON(resp.Body, &results); err != nil {
		return nil, nil, errors.Wrap(err, "reading response")
	}

	return results.Order, results.Logic, nil
}

// FindHostByIpAddress queries the database for the host with ip matching the ip address
func (c *communicatorImpl) FindHostByIpAddress(ctx context.Context, ip string) (*model.APIHost, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("hosts/ip_address/%s", ip),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error sending request to find host by ip address")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, AuthError
	}
	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting hosts")
	}

	host := &model.APIHost{}
	if err = utility.ReadJSON(resp.Body, host); err != nil {
		return nil, errors.Wrap(err, "can't read response as APIHost")
	}
	return host, nil
}
