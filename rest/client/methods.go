package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
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
		return nil, errors.Wrapf(err, "sending request to create spawn host")
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "creating spawn host")
	}

	spawnHostResp := model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &spawnHostResp); err != nil {
		return nil, errors.Wrap(err, "reading response body")
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
		return nil, errors.Wrapf(err, "sending request to get spawn host '%s'", hostId)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting spawn host '%s'", hostId)
	}

	spawnHostResp := model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &spawnHostResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to modify spawn host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "modifying spawn host '%s'", hostID)
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
		return errors.Wrapf(err, "sending request to stop spawn host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "stopping spawn host '%s'", hostID)
	}

	if wait {
		return errors.Wrap(c.waitForStatus(ctx, hostID, evergreen.HostStopped), "waiting for spawn host to stop")
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
		return errors.Wrapf(err, "sending request to attach volume to host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "attaching volume to host '%s'", hostID)
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
		return errors.Wrapf(err, "sending request to detach volume '%s' from host '%s'", volumeID, hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "detaching volume '%s' from host '%s'", volumeID, hostID)
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
		return nil, errors.Wrap(err, "sending request to create volume")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "creating volume")
	}

	createVolumeResp := model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, &createVolumeResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to delete volume '%s'", volumeID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "deleting volume '%s'", volumeID)
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
		return errors.Wrapf(err, "sending request to modify volume '%s'", volumeID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "modifying volume '%s'", volumeID)
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
		return nil, errors.Wrapf(err, "sending request to get volume '%s'", volumeID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting volume '%s'", volumeID)
	}

	volumeResp := &model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, volumeResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to get volumes for user '%s'", c.apiUser)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting volumes for user '%s'", c.apiUser)
	}

	getVolumesResp := []model.APIVolume{}
	if err = utility.ReadJSON(resp.Body, &getVolumesResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to start spawn host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "starting host '%s'", hostID)
	}

	if wait {
		return errors.Wrap(c.waitForStatus(ctx, hostID, evergreen.HostRunning), "waiting for host to start")
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
			return errors.Wrap(timerCtx.Err(), "timer context canceled")
		case <-timer.C:
			resp, err := c.request(ctx, info, "")
			if err != nil {
				return errors.Wrapf(err, "sending request to get info for host '%s'", hostID)
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				return util.RespErrorf(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return util.RespErrorf(resp, "getting host status")
			}
			hostResp := model.APIHost{}
			if err = utility.ReadJSON(resp.Body, &hostResp); err != nil {
				return errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to terminate host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "terminating host '%s'", hostID)
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
		return errors.Wrapf(err, "sending request to change RDP password for host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "changing RDP password for host '%s'", hostID)
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
		return errors.Wrapf(err, "sending request to extend expiration of host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "changing expiration of host '%s'", hostID)
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
		return nil, errors.Wrap(err, "sending request to get hosts")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting hosts")
	}

	hosts := []*model.APIHost{}
	if err = utility.ReadJSON(resp.Body, &hosts); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return util.RespErrorf(resp, errors.Wrap(err, "sending request to set banner message").Error())
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
		return "", errors.Wrap(err, "sending request to get current banner message")
	}
	defer resp.Body.Close()

	banner := model.APIBanner{}
	if err = utility.ReadJSON(resp.Body, &banner); err != nil {
		return "", errors.Wrap(err, "reading JSON response body")
	}

	return utility.FromStringPtr(banner.Text), nil
}

func (c *communicatorImpl) GetUiV2URL(ctx context.Context) (string, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "admin/uiv2_url",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return "", errors.Wrap(err, "sending request to get current UI v2 URL")
	}
	defer resp.Body.Close()

	uiV2 := model.APIUiV2URL{}
	if err = utility.ReadJSON(resp.Body, &uiV2); err != nil {
		return "", errors.Wrap(err, "reading JSON response body")
	}

	return utility.FromStringPtr(uiV2.UIv2Url), nil
}

func (c *communicatorImpl) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   "admin/service_flags",
	}

	resp, err := c.retryRequest(ctx, info, f)
	if err != nil {
		return util.RespErrorf(resp, errors.Wrap(err, "sending request to set service flags").Error())
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
		return nil, errors.Wrap(err, "sending request to get service flags")
	}
	defer resp.Body.Close()

	settings := model.APIAdminSettings{}
	if err = utility.ReadJSON(resp.Body, &settings); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	return settings.ServiceFlags, nil
}

func (c *communicatorImpl) RestartRecentTasks(ctx context.Context, startAt, endAt time.Time) error {
	if endAt.Before(startAt) {
		return errors.Errorf("start time %s cannot be before end time %s", startAt, endAt)
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
		return errors.Wrap(err, "sending request to restart recent tasks")
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
		return nil, errors.Wrap(err, "sending request to get admin settings")
	}
	defer resp.Body.Close()

	settings := &evergreen.Settings{}

	if err = utility.ReadJSON(resp.Body, settings); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrap(err, "sending request to update admin settings")
	}
	defer resp.Body.Close()

	newSettings := &model.APIAdminSettings{}
	err = utility.ReadJSON(resp.Body, newSettings)
	if err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrap(err, "sending request to get admin events")
	}
	defer resp.Body.Close()

	events := []interface{}{}
	err = utility.ReadJSON(resp.Body, &events)
	if err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to revert admin settings for event '%s'", guid)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("reverting event '%s'", guid)
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
		return nil, errors.Wrapf(err, "sending request to execute script on hosts in distro '%s'", distro)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "running script on hosts in distro '%s'", distro)
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
		return nil, errors.Wrap(err, "sending request to get service users")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting service users")
	}
	var result []model.APIDBUser
	if err = utility.ReadJSON(resp.Body, &result); err != nil {
		return nil, errors.Wrap(err, "reading JSON response")
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
		return errors.Wrapf(err, "sending request to update service user '%s'", username)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "updating service user '%s'", username)
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
		return errors.Wrapf(err, "sending request to delete service user '%s'", username)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "deleting service user '%s'", username)
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
		return nil, errors.Wrap(err, "sending request to get distros")
	}
	defer resp.Body.Close()

	distros := []model.APIDistro{}

	if err = utility.ReadJSON(resp.Body, &distros); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to get public keys for user '%s'", c.apiUser)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting public keys for user '%s'", c.apiUser)
	}

	keys := []model.APIPubKey{}

	if err = utility.ReadJSON(resp.Body, &keys); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrapf(err, "sending request to add public key '%s'", keyName)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "adding public key '%s'", keyName)
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
		return errors.Wrapf(err, "sending request to delete public key '%s'", keyName)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "deleting public key '%s'", keyName)
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
		return nil, errors.Wrap(err, "sending request to list project aliases")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "listing project aliases")
	}
	patchAliases := []serviceModel.ProjectAlias{}

	// use io.ReadAll and json.Unmarshal instead of utility.ReadJSON since we may read the results twice
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	if err := json.Unmarshal(bytes, &patchAliases); err != nil {
		patchAlias := serviceModel.ProjectAlias{}
		if err := json.Unmarshal(bytes, &patchAlias); err != nil {
			return nil, errors.Wrap(err, "unmarshalling JSON response body into patch alias")
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
		return nil, errors.Wrap(err, "sending request to list patch trigger aliases")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "listing patch trigger aliases")
	}

	triggerAliases := []string{}
	if err = utility.ReadJSON(resp.Body, &triggerAliases); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrap(err, "sending request to get latest CLI version information")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting latest CLI version information")
	}
	update := &model.APICLIUpdate{}
	if err = utility.ReadJSON(resp.Body, update); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	config := update.ClientConfig.ToService()
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
		return nil, errors.Wrapf(err, "sending request to get patch parameters for project '%s'", project)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting patch parameters for project '%s'", project)
	}

	params := []serviceModel.ParameterInfo{}
	if err = utility.ReadJSON(resp.Body, &params); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "getting subscriptions for user '%s'", c.apiUser)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting subscriptions for user '%s'", c.apiUser)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	apiSubs := []model.APISubscription{}
	if err = json.Unmarshal(bytes, &apiSubs); err != nil {
		apiSub := model.APISubscription{}
		if err = json.Unmarshal(bytes, &apiSub); err != nil {
			return nil, errors.Wrap(err, "unmarshalling JSON response body into subscriptions")
		}

		apiSubs = append(apiSubs, apiSub)
	}

	subs := make([]event.Subscription, len(apiSubs))
	for i := range apiSubs {
		subs[i], err = apiSubs[i].ToService()
		if err != nil {
			return nil, errors.Wrap(err, "converting API model")
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
		ProjectID string `json:"project_id"`
		Message   string `json:"message"`
		Active    bool   `json:"activate"`
		IsAdHoc   bool   `json:"is_adhoc"`
		Config    []byte `json:"config"`
	}{
		ProjectID: project,
		Message:   message,
		Active:    active,
		IsAdHoc:   true,
		Config:    config,
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return nil, errors.Wrap(err, "sending request to create version from config")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "creating version from config")
	}

	v := &serviceModel.Version{}
	if err = utility.ReadJSON(resp.Body, v); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to get commit queue for project '%s'", projectID)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting commit queue for project '%s'", projectID)
	}

	cq := model.APICommitQueue{}
	if err = utility.ReadJSON(resp.Body, &cq); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	return &cq, nil
}

func (c *communicatorImpl) DeleteCommitQueueItem(ctx context.Context, item string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   fmt.Sprintf("/commit_queue/%s", item),
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return errors.Wrapf(err, "deleting item '%s' from commit queue", item)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return util.RespErrorf(resp, "deleting item '%s' from commit queue", item)
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
		return 0, errors.Wrapf(err, "sending request to enqueue item '%s'", patchID)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return 0, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, util.RespErrorf(resp, "enqueueing commit queue item '%s'", patchID)
	}

	positionResp := model.APICommitQueuePosition{}
	if err = utility.ReadJSON(resp.Body, &positionResp); err != nil {
		return 0, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to create merge patch '%s'", patchID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "creating merge patch '%s'", patchID)
	}

	newPatch := &model.APIPatch{}
	if err := utility.ReadJSON(resp.Body, newPatch); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return "", errors.Wrapf(err, "sending request to get message for patch '%s'", patchID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", util.RespErrorf(resp, "getting message for patch '%s'", patchID)
	}

	var message string
	if err := utility.ReadJSON(resp.Body, &message); err != nil {
		return "", errors.Wrap(err, "reading JSON response body")
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
		return errors.Wrap(err, "sending request to send notification")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "sending notification")
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
		return nil, errors.Wrapf(err, "sending request to get status for container '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting status for container '%s'", hostID)
	}
	status := cloud.ContainerStatus{}
	if err := utility.ReadJSON(resp.Body, &status); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to get logs for container '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting logs for container '%s'", hostID)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to get manifest for task '%s'", taskId)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting manifest for task '%s'", taskId)
	}
	mfest := manifest.Manifest{}
	if err := utility.ReadJSON(resp.Body, &mfest); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
				return nil, errors.Wrap(err, "sending request to run process on hosts")
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusUnauthorized {
				return nil, util.RespErrorf(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, util.RespErrorf(resp, "running process on hosts")
			}

			output := []model.APIHostProcess{}
			if err := utility.ReadJSON(resp.Body, &output); err != nil {
				return nil, errors.Wrap(err, "reading JSON response body")
			}

			return output, nil
		}()
		if err != nil {
			return nil, err
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
				return nil, errors.Wrap(err, "sending request to get process output from hosts")
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusUnauthorized {
				return nil, util.RespErrorf(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, util.RespErrorf(resp, "getting process output from hosts")
			}

			output := []model.APIHostProcess{}
			if err := utility.ReadJSON(resp.Body, &output); err != nil {
				return nil, errors.Wrap(err, "reading JSON response body")
			}

			return output, nil
		}()
		if err != nil {
			return nil, err
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
		return nil, errors.Wrapf(err, "sending request to get versions for project '%s' and requester '%s'", projectID, requester)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting versions for project '%s' and requester '%s'", projectID, requester)
	}

	getVersionsResp := []model.APIVersion{}
	if err = utility.ReadJSON(resp.Body, &getVersionsResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrap(err, "sending request to get task sync read-only credentials")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting task sync read-only credentials")
	}
	creds := &evergreen.S3Credentials{}
	if err := utility.ReadJSON(resp.Body, creds); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return "", errors.Wrap(err, "sending request to get task sync path")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return "", util.RespErrorf(resp, "getting task sync path")
	}
	path, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body")
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
		return nil, util.RespErrorf(resp, errors.Wrapf(err, "getting distro '%s'", id).Error())
	}
	defer resp.Body.Close()

	d := &restmodel.APIDistro{}
	if err = utility.ReadJSON(resp.Body, &d); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, util.RespErrorf(resp, errors.Wrap(err, "getting client URLs").Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting client URLs")
	}

	var urls []string
	if err := utility.ReadJSON(resp.Body, &urls); err != nil {
		return nil, errors.Wrapf(err, "reading JSON response body")
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
		return nil, util.RespErrorf(resp, "sending request to get provisioning options for host '%s'", hostID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting host provisioning options")
	}
	var opts restmodel.APIHostProvisioningOptions
	if err = utility.ReadJSON(resp.Body, &opts); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, nil, errors.Wrap(err, "creating request")
	}
	resp, err := c.doRequest(ctx, r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "sending request to get task comparison")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, util.RespErrorf(resp, "getting task comparison")
	}
	var results restmodel.CompareTasksResponse
	if err = utility.ReadJSON(resp.Body, &results); err != nil {
		return nil, nil, errors.Wrap(err, "reading JSON response body")
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
		return nil, errors.Wrapf(err, "sending request to find host by IP address")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting host by IP address")
	}

	host := &model.APIHost{}
	if err = utility.ReadJSON(resp.Body, host); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}
	return host, nil
}

// GetRawPatchWithModules fetches the raw patch and module diffs for a given patch ID.
func (c *communicatorImpl) GetRawPatchWithModules(ctx context.Context, patchId string) (*restmodel.APIRawPatch, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("patches/%s/raw_modules", patchId),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to find host by IP address")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespErrorf(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting host by IP address")
	}

	rp := restmodel.APIRawPatch{}
	if err = utility.ReadJSON(resp.Body, &rp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}
	return &rp, nil
}
