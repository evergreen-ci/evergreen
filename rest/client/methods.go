package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/kanopy-platform/kanopy-oidc-lib/pkg/dex"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

const (
	refreshTokenClaimed = "claimed by another client"
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "creating spawn host")
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
		return nil, util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "modifying spawn host '%s'", hostID)
	}

	return nil
}

func (c *communicatorImpl) StopSpawnHost(ctx context.Context, hostID string, subscriptionType string, shouldKeepOff, wait bool) error {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("hosts/%s/stop", hostID),
	}

	options := struct {
		SubscriptionType string `json:"subscription_type"`
		ShouldKeepOff    bool   `json:"should_keep_off"`
	}{
		SubscriptionType: subscriptionType,
		ShouldKeepOff:    shouldKeepOff,
	}

	resp, err := c.request(ctx, info, options)
	if err != nil {
		return errors.Wrapf(err, "sending request to stop spawn host '%s'", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "creating volume")
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return nil, util.RespError(resp, AuthError)
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
		return nil, util.RespError(resp, AuthError)
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

// getUser gets information about a user by their user ID.
func (c *communicatorImpl) getUser(ctx context.Context, userID string) (*model.APIDBUser, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("users/%s", userID),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error sending request to get user '%s'", userID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("HTTP request returned unexpected status: %d", resp.StatusCode)
	}

	user := &model.APIDBUser{}
	if err = utility.ReadJSON(resp.Body, user); err != nil {
		return nil, errors.Wrap(err, "error reading JSON response body")
	}

	return user, nil
}

// IsServiceUser checks if the given user is a service user (has OnlyApi flag set).
func (c *communicatorImpl) IsServiceUser(ctx context.Context, userID string) (bool, error) {
	user, err := c.getUser(ctx, userID)
	if err != nil {
		return false, err
	}

	if user == nil {
		return false, errors.Errorf("user '%s' not found", userID)
	}

	return user.OnlyApi, nil
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
		return util.RespError(resp, AuthError)
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
				return util.RespError(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return util.RespError(resp, "getting host status")
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting hosts")
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
		return util.RespError(resp, errors.Wrap(err, "sending request to set banner message").Error())
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

// GetEstimatedGeneratedTasks returns the estimated number of generated tasks to be created by an unfinalized patch.
func (c *communicatorImpl) GetEstimatedGeneratedTasks(ctx context.Context, patchId string, tvPairs []serviceModel.TVPair) (int, error) {
	if len(tvPairs) == 0 {
		return 0, nil
	}
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("patches/%s/estimated_generated_tasks", patchId),
	}
	resp, err := c.request(ctx, info, tvPairs)
	if err != nil {
		return 0, errors.Wrap(err, "sending request to estimate number of activated generated tasks")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("getting an estimated number of activated generated tasks")
	}

	numTasksToFinalize := model.APINumTasksToFinalize{}
	if err = utility.ReadJSON(resp.Body, &numTasksToFinalize); err != nil {
		return 0, errors.Wrap(err, "reading JSON response body")
	}

	return utility.FromIntPtr(numTasksToFinalize.NumTasksToFinalize), nil
}

func (c *communicatorImpl) RevokeGitHubDynamicAccessTokens(ctx context.Context, taskId string, tokens []string) error {
	info := requestInfo{
		method: http.MethodDelete,
		path:   fmt.Sprintf("tasks/%s/github_dynamic_access_tokens", taskId),
	}

	resp, err := c.request(ctx, info, tokens)
	if err != nil {
		return errors.Wrap(err, "revoking GitHub dynamic access token")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.RespError(resp, "revoking GitHub dynamic access token")
	}

	return nil
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
		return util.RespError(resp, errors.Wrap(err, "sending request to set service flags").Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   "admin/service_flags",
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error sending request to get service flags")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("HTTP request returned unexpected status: %d", resp.StatusCode)
	}

	flags := &model.APIServiceFlags{}
	if err = utility.ReadJSON(resp.Body, flags); err != nil {
		return nil, errors.Wrap(err, "error reading JSON response body")
	}

	return flags, nil
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

func (c *communicatorImpl) GetEvents(ctx context.Context, ts time.Time, limit int) ([]any, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("admin/events?ts=%s&limit=%d", ts.Format(time.RFC3339), limit),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "sending request to get admin events")
	}
	defer resp.Body.Close()

	events := []any{}
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
		return util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("reverting event '%s'", guid)
	}

	return nil
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting service users")
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting all distros")
	}

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
		return nil, util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
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
		return util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespErrorf(resp, "deleting public key '%s'", keyName)
	}

	return nil
}

func (c *communicatorImpl) ListAliases(ctx context.Context, project string, includeProjectConfig bool) ([]serviceModel.ProjectAlias, error) {
	path := fmt.Sprintf("alias/%s", project)
	info := requestInfo{
		method: http.MethodGet,
		path:   path,
	}
	if includeProjectConfig {
		info.path += "?includeProjectConfig=true"
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrap(err, "sending request to list project aliases")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "listing project aliases")
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "listing patch trigger aliases")
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting latest CLI version information")
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
		return nil, util.RespError(resp, AuthError)
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
		return nil, util.RespError(resp, AuthError)
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

func (c *communicatorImpl) SendNotification(ctx context.Context, notificationType string, data any) error {
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
		return util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return util.RespError(resp, "sending notification")
	}

	return nil
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
		return nil, util.RespError(resp, AuthError)
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
				return nil, util.RespError(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, util.RespError(resp, "running process on hosts")
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
				return nil, util.RespError(resp, AuthError)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, util.RespError(resp, "getting process output from hosts")
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

func (c *communicatorImpl) GetRecentVersionsForProject(ctx context.Context, projectID, requester string, startAtOrderNum, limit int) ([]model.APIVersion, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("projects/%s/versions", projectID),
	}
	queryParams := []string{}
	if requester != "" {
		queryParams = append(queryParams, fmt.Sprintf("requester=%s", requester))
	}
	if startAtOrderNum > 0 {
		queryParams = append(queryParams, fmt.Sprintf("start=%d", startAtOrderNum))
	}
	if limit > 0 {
		queryParams = append(queryParams, fmt.Sprintf("limit=%d", limit))
	}
	if len(queryParams) > 0 {
		info.path = info.path + "?" + strings.Join(queryParams, "&")
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get versions for project '%s'", projectID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting versions for project '%s'", projectID)
	}

	getVersionsResp := []model.APIVersion{}
	if err = utility.ReadJSON(resp.Body, &getVersionsResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	return getVersionsResp, nil
}

func (c *communicatorImpl) GetBuildsForVersion(ctx context.Context, versionID string) ([]restmodel.APIBuild, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("versions/%s/builds", versionID),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get builds for version '%s'", versionID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting builds for version '%s'", versionID)
	}

	getBuildsResp := []model.APIBuild{}
	if err = utility.ReadJSON(resp.Body, &getBuildsResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	return getBuildsResp, nil
}

func (c *communicatorImpl) GetTasksForBuild(ctx context.Context, buildID string, startAt string, limit int) ([]restmodel.APITask, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("builds/%s/tasks", buildID),
	}
	var queryParams []string
	if startAt != "" {
		queryParams = append(queryParams, fmt.Sprintf("start_at=%s", startAt))
	}
	if limit > 0 {
		queryParams = append(queryParams, fmt.Sprintf("limit=%d", limit))
	}
	if len(queryParams) > 0 {
		info.path = info.path + "?" + strings.Join(queryParams, "&")
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get tasks for build '%s'", buildID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "getting tasks for build '%s'", buildID)
	}

	getTasksResp := []model.APITask{}
	if err = utility.ReadJSON(resp.Body, &getTasksResp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}

	return getTasksResp, nil
}

func (c *communicatorImpl) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("distros/%s", id),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, util.RespError(resp, errors.Wrapf(err, "getting distro '%s'", id).Error())
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
		return nil, util.RespError(resp, errors.Wrap(err, "getting client URLs").Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting client URLs")
	}

	var urls []string
	if err := utility.ReadJSON(resp.Body, &urls); err != nil {
		return nil, errors.Wrapf(err, "reading JSON response body")
	}

	return urls, nil
}

func (c *communicatorImpl) PostHostIsUp(ctx context.Context, ec2InstanceID, hostname string) (*restmodel.APIHost, error) {
	info := requestInfo{
		method: http.MethodPost,
		path:   fmt.Sprintf("/hosts/%s/is_up", c.hostID),
	}
	opts := restmodel.APIHostIsUpOptions{
		HostID:        c.hostID,
		Hostname:      hostname,
		EC2InstanceID: ec2InstanceID,
	}
	r, err := c.createRequest(info, opts)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}
	resp, err := utility.RetryRequest(ctx, r, utility.RetryRequestOptions{
		RetryOptions: utility.RetryOptions{
			MaxAttempts: c.maxAttempts,
			MinDelay:    c.timeoutStart,
			MaxDelay:    c.timeoutMax,
		},
	})
	if err != nil {
		return nil, util.RespError(resp, errors.Wrapf(err, "sending request to indicate host '%s' is up", c.hostID).Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(resp, "posting that host '%s' is up", c.hostID)
	}
	var h restmodel.APIHost
	if err = utility.ReadJSON(resp.Body, &h); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}
	return &h, nil
}

func (c *communicatorImpl) GetHostProvisioningOptions(ctx context.Context) (*restmodel.APIHostProvisioningOptions, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("/hosts/%s/provisioning_options", c.hostID),
	}
	r, err := c.createRequest(info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating request")
	}
	resp, err := utility.RetryRequest(ctx, r, utility.RetryRequestOptions{
		RetryOptions: utility.RetryOptions{
			MaxAttempts: c.maxAttempts,
			MinDelay:    c.timeoutStart,
			MaxDelay:    c.timeoutMax,
		},
	})
	if err != nil {
		return nil, util.RespError(resp, errors.Wrapf(err, "sending request to get provisioning options for host '%s'", c.hostID).Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting host provisioning options")
	}
	var opts restmodel.APIHostProvisioningOptions
	if err = utility.ReadJSON(resp.Body, &opts); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}
	return &opts, nil
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
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting host by IP address")
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
		return nil, errors.Wrapf(err, "sending request to get raw patch with modules")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting raw patch with modules")
	}

	rp := restmodel.APIRawPatch{}
	if err = utility.ReadJSON(resp.Body, &rp); err != nil {
		return nil, errors.Wrap(err, "reading JSON response body")
	}
	return &rp, nil
}

func (c *communicatorImpl) GetManifestForVersion(ctx context.Context, versionID string) (*restmodel.APIManifest, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("versions/%s/manifest", versionID),
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get version manifest")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode == http.StatusNotFound {
		// Manifests are optional for versions that don't use modules, so the
		// route can return 404 if the version does not exist or if the version
		// has no manifest.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting version manifest")
	}

	manifestResp := restmodel.APIManifest{}
	if err = utility.ReadJSON(resp.Body, &manifestResp); err != nil {
		return nil, errors.Wrap(err, "reading manifest response body")
	}
	return &manifestResp, nil
}

func (c *communicatorImpl) GetTaskLogs(ctx context.Context, opts GetTaskLogsOptions) (io.ReadCloser, error) {
	var params []string
	if opts.Execution != nil {
		params = append(params, fmt.Sprintf("execution=%d", utility.FromIntPtr(opts.Execution)))
	}
	if opts.Type != "" {
		params = append(params, fmt.Sprintf("type=%s", opts.Type))
	}
	if opts.Start != "" {
		params = append(params, fmt.Sprintf("start=%s", opts.Start))
	}
	if opts.End != "" {
		params = append(params, fmt.Sprintf("end=%s", opts.End))
	}
	if opts.LineLimit > 0 {
		params = append(params, fmt.Sprintf("line_limit=%d", opts.LineLimit))
	}
	if opts.TailLimit > 0 {
		params = append(params, fmt.Sprintf("tail_limit=%d", opts.TailLimit))
	}
	if opts.PrintTime {
		params = append(params, fmt.Sprintf("print_time=%v", opts.PrintTime))
	}
	if opts.PrintPriority {
		params = append(params, fmt.Sprintf("print_priority=%v", opts.PrintPriority))
	}
	if opts.Paginate {
		params = append(params, fmt.Sprintf("paginate=%v", opts.Paginate))
	}

	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("tasks/%s/build/TaskLogs?%s", opts.TaskID, strings.Join(params, "&")),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get task logs")
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting task logs")
	}

	header := make(http.Header)
	if c.oauth != "" {
		header.Add(evergreen.AuthorizationHeader, "Bearer "+c.oauth)
	} else if c.apiUser != "" && c.apiKey != "" {
		header.Add(evergreen.APIUserHeader, c.apiUser)
		header.Add(evergreen.APIKeyHeader, c.apiKey)
	}
	return utility.NewPaginatedReadCloser(ctx, c.httpClient, resp, header), nil
}

func (c *communicatorImpl) GetTestLogs(ctx context.Context, opts GetTestLogsOptions) (io.ReadCloser, error) {
	var params []string
	if opts.Execution != nil {
		params = append(params, fmt.Sprintf("execution=%d", utility.FromIntPtr(opts.Execution)))
	}
	for _, path := range opts.LogsToMerge {
		params = append(params, fmt.Sprintf("logs_to_merge=%s", url.QueryEscape(path)))
	}
	if opts.Start != "" {
		params = append(params, fmt.Sprintf("start=%s", opts.Start))
	}
	if opts.End != "" {
		params = append(params, fmt.Sprintf("end=%s", opts.End))
	}
	if opts.LineLimit > 0 {
		params = append(params, fmt.Sprintf("line_limit=%d", opts.LineLimit))
	}
	if opts.TailLimit > 0 {
		params = append(params, fmt.Sprintf("tail_limit=%d", opts.TailLimit))
	}
	if opts.PrintTime {
		params = append(params, fmt.Sprintf("print_time=%v", opts.PrintTime))
	}
	if opts.PrintPriority {
		params = append(params, fmt.Sprintf("print_priority=%v", opts.PrintPriority))
	}
	if opts.Paginate {
		params = append(params, fmt.Sprintf("paginate=%v", opts.Paginate))
	}
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("tasks/%s/build/TestLogs/%s?%s", opts.TaskID, url.PathEscape(opts.Path), strings.Join(params, "&")),
	}

	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "sending request to get test logs")
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, util.RespError(resp, AuthError)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "getting test logs")
	}

	header := make(http.Header)
	// The API user and key are mutually exclusive with JWT, so only set them if
	// they are both set.
	if c.oauth != "" {
		header.Add(evergreen.AuthorizationHeader, "Bearer "+c.oauth)
	} else if c.apiUser != "" && c.apiKey != "" {
		header.Add(evergreen.APIUserHeader, c.apiUser)
		header.Add(evergreen.APIKeyHeader, c.apiKey)
	}
	return utility.NewPaginatedReadCloser(ctx, c.httpClient, resp, header), nil
}

const server400 = "server returned status 400"

func (c *communicatorImpl) Validate(ctx context.Context, data []byte, quiet bool, projectID string) (validator.ValidationErrors, error) {
	// 413 errors are transient when validating large project configurations
	// so we want to retry on them.
	info := requestInfo{
		method:     http.MethodPost,
		path:       "validate",
		retryOn413: true,
	}

	body := validator.ValidationInput{
		ProjectYaml: data,
		Quiet:       quiet,
		ProjectID:   projectID,
	}
	resp, err := c.retryRequest(ctx, info, body)
	if resp != nil {
		defer resp.Body.Close()
	}

	// we want to ignore the error if it's a 400, since that is expected when validation fails
	if err != nil && err.Error() != server400 {
		return nil, util.RespError(resp, "validating project")
	}

	if resp.StatusCode == http.StatusBadRequest {
		rawData, _ := io.ReadAll(resp.Body)

		var errorResponse gimlet.ErrorResponse
		err := json.Unmarshal(rawData, &errorResponse)
		if err == nil {
			// if it successfully unmarshaled to a gimlet error response,
			// that means it's an error message rather than project errors and
			// we should return the error.
			return nil, errors.New(errorResponse.Message)
		}

		errors := validator.ValidationErrors{}
		err = json.Unmarshal(rawData, &errors)
		if err != nil {
			return nil, util.RespError(resp, "reading validation errors")
		}
		return errors, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, util.RespError(resp, "validating project")
	}

	return nil, nil
}

func (c *communicatorImpl) GetOAuthToken(ctx context.Context, loader dex.TokenLoader, opts ...dex.ClientOption) (*oauth2.Token, error) {
	httpClient := utility.GetDefaultHTTPRetryableClient()
	defer utility.PutHTTPClient(httpClient)
	ctx = oidc.ClientContext(ctx, httpClient)

	opts = append(opts,
		dex.WithContext(ctx),
		dex.WithRefresh(),
	)

	// The Dex client logs using logrus. The client doesn't
	// have any way to turn off debug logs within it's API.
	// We set the output to io.Discard to suppress debug logs.
	logrus.SetOutput(io.Discard)

	client, err := dex.NewClient(append(opts, dex.WithTokenLoader(loader))...)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// This attempt tries to get a token or refresh using the refresh token.
	token, err := client.Token()
	if err == nil {
		return token, nil
	}
	// Sometimes, the refresh token is invalid or claimed by another client.
	// In this case, we need to run through the auth flow again without using
	// the refresh token.
	if !strings.Contains(err.Error(), refreshTokenClaimed) {
		return nil, err
	}

	// This client prevents the Dex client from using the refresh token.
	client, err = dex.NewClient(append(opts, dex.WithTokenLoader(&tokenLoaderWithoutRefresh{loader}))...)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	return client.Token()
}

type tokenLoaderWithoutRefresh struct {
	dex.TokenLoader
}

func (t *tokenLoaderWithoutRefresh) LoadToken(_ string) (*oauth2.Token, error) {
	token, err := t.TokenLoader.LoadToken("")
	if err != nil {
		return nil, err
	}
	// Clear the refresh token to prevent the Dex client from trying to use it.
	token.RefreshToken = ""
	return token, nil
}
