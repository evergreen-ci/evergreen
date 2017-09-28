package client

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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

func (c *communicatorImpl) SetBannerMessage(ctx context.Context, m string) error {
	info := requestInfo{
		method:  post,
		version: apiVersion2,
		path:    "admin/banner",
	}

	_, err := c.retryRequest(ctx, info, struct {
		Banner string `json:"banner"`
	}{
		Banner: m,
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

// nolint
func (*communicatorImpl) GetTaskByID()                        {}
func (*communicatorImpl) GetTasksByBuild()                    {}
func (*communicatorImpl) GetTasksByProjectAndCommit()         {}
func (*communicatorImpl) SetTaskStatus()                      {}
func (*communicatorImpl) AbortTask()                          {}
func (*communicatorImpl) RestartTask()                        {}
func (*communicatorImpl) GetKeys()                            {}
func (*communicatorImpl) AddKey()                             {}
func (*communicatorImpl) RemoveKey()                          {}
func (*communicatorImpl) GetProjectByID()                     {}
func (*communicatorImpl) EditProject()                        {}
func (*communicatorImpl) CreateProject()                      {}
func (*communicatorImpl) GetAllProjects()                     {}
func (*communicatorImpl) GetBuildByID()                       {}
func (*communicatorImpl) GetBuildByProjectAndHashAndVariant() {}
func (*communicatorImpl) GetBuildsByVersion()                 {}
func (*communicatorImpl) SetBuildStatus()                     {}
func (*communicatorImpl) AbortBuild()                         {}
func (*communicatorImpl) RestartBuild()                       {}
func (*communicatorImpl) GetTestsByTaskID()                   {}
func (*communicatorImpl) GetTestsByBuild()                    {}
func (*communicatorImpl) GetTestsByTestName()                 {}
func (*communicatorImpl) GetVersionByID()                     {}
func (*communicatorImpl) GetVersions()                        {}
func (*communicatorImpl) GetVersionByProjectAndCommit()       {}
func (*communicatorImpl) GetVersionsByProject()               {}
func (*communicatorImpl) SetVersionStatus()                   {}
func (*communicatorImpl) AbortVersion()                       {}
func (*communicatorImpl) RestartVersion()                     {}
func (*communicatorImpl) GetAllDistros()                      {}
func (*communicatorImpl) GetDistroByID()                      {}
func (*communicatorImpl) CreateDistro()                       {}
func (*communicatorImpl) EditDistro()                         {}
func (*communicatorImpl) DeleteDistro()                       {}
func (*communicatorImpl) GetDistroSetupScriptByID()           {}
func (*communicatorImpl) GetDistroTeardownScriptByID()        {}
func (*communicatorImpl) EditDistroSetupScript()              {}
func (*communicatorImpl) EditDistroTeardownScript()           {}
func (*communicatorImpl) GetPatchByID()                       {}
func (*communicatorImpl) GetPatchesByProject()                {}
func (*communicatorImpl) SetPatchStatus()                     {}
func (*communicatorImpl) AbortPatch()                         {}
func (*communicatorImpl) RestartPatch()                       {}
