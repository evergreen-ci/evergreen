package client

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// GetAllHosts ...
func (*communicatorImpl) GetAllHosts() {
	return
}

// GetHostByID ...
func (*communicatorImpl) GetHostByID() {
	return
}

// SetHostStatus ...
func (*communicatorImpl) SetHostStatus() {
	return
}

// SetHostStatuses ...
func (*communicatorImpl) SetHostStatuses() {
	return
}

// CreateSpawnHost will insert an intent host into the DB that will be spawned later by the runner
func (c *communicatorImpl) CreateSpawnHost(ctx context.Context, distroID string, keyName string) (*model.SpawnHost, error) {
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

	spawnHostResp := model.SpawnHost{}
	defer resp.Body.Close()
	if err = util.ReadJSONInto(resp.Body, &spawnHostResp); err != nil {
		return nil, fmt.Errorf("Error forming response body response: %v", err)
	}
	return &spawnHostResp, nil
}

// GetSpawnHosts ...
func (*communicatorImpl) GetSpawnHosts() {
	return
}

// GetTaskByID ...
func (*communicatorImpl) GetTaskByID() {
	return
}

// GetTasksByBuild ...
func (*communicatorImpl) GetTasksByBuild() {
	return
}

// GetTasksByProjectAndCommit ...
func (*communicatorImpl) GetTasksByProjectAndCommit() {
	return
}

// SetTaskStatus ...
func (*communicatorImpl) SetTaskStatus() {
	return
}

// AbortTask ...
func (*communicatorImpl) AbortTask() {
	return
}

// RestartTask ...
func (*communicatorImpl) RestartTask() {
	return
}

// GetKeys ...
func (*communicatorImpl) GetKeys() {
	return
}

// AddKey ...
func (*communicatorImpl) AddKey() {
	return
}

// RemoveKey ...
func (*communicatorImpl) RemoveKey() {
	return
}

// GetProjectByID ...
func (*communicatorImpl) GetProjectByID() {
	return
}

// EditProject ...
func (*communicatorImpl) EditProject() {
	return
}

// CreateProject ...
func (*communicatorImpl) CreateProject() {
	return
}

// GetAllProjects ...
func (*communicatorImpl) GetAllProjects() {
	return
}

// GetBuildByID ...
func (*communicatorImpl) GetBuildByID() {
	return
}

// GetBuildByProjectAndHashAndVariant ...
func (*communicatorImpl) GetBuildByProjectAndHashAndVariant() {
	return
}

// GetBuildsByVersion ...
func (*communicatorImpl) GetBuildsByVersion() {
	return
}

// SetBuildStatus ...
func (*communicatorImpl) SetBuildStatus() {
	return
}

// AbortBuild ...
func (*communicatorImpl) AbortBuild() {
	return
}

// RestartBuild ...
func (*communicatorImpl) RestartBuild() {
	return
}

// GetTestsByTaskID ...
func (*communicatorImpl) GetTestsByTaskID() {
	return
}

// GetTestsByBuild ...
func (*communicatorImpl) GetTestsByBuild() {
	return
}

// GetTestsByTestName ...
func (*communicatorImpl) GetTestsByTestName() {
	return
}

// GetVersionByID ...
func (*communicatorImpl) GetVersionByID() {
	return
}

// GetVersions ...
func (*communicatorImpl) GetVersions() {
	return
}

// GetVersionByProjectAndCommit ...
func (*communicatorImpl) GetVersionByProjectAndCommit() {
	return
}

// GetVersionsByProject ...
func (*communicatorImpl) GetVersionsByProject() {
	return
}

// SetVersionStatus ...
func (*communicatorImpl) SetVersionStatus() {
	return
}

// AbortVersion ...
func (*communicatorImpl) AbortVersion() {
	return
}

// RestartVersion ...
func (*communicatorImpl) RestartVersion() {
	return
}

// GetAllDistros ...
func (*communicatorImpl) GetAllDistros() {
	return
}

// GetDistroByID ...
func (*communicatorImpl) GetDistroByID() {
	return
}

// CreateDistro ...
func (*communicatorImpl) CreateDistro() {
	return
}

// EditDistro ...
func (*communicatorImpl) EditDistro() {
	return
}

// DeleteDistro ...
func (*communicatorImpl) DeleteDistro() {
	return
}

// GetDistroSetupScriptByID ...
func (*communicatorImpl) GetDistroSetupScriptByID() {
	return
}

// GetDistroTeardownScriptByID ...
func (*communicatorImpl) GetDistroTeardownScriptByID() {
	return
}

// EditDistroSetupScript ...
func (*communicatorImpl) EditDistroSetupScript() {
	return
}

// EditDistroTeardownScript ...
func (*communicatorImpl) EditDistroTeardownScript() {
	return
}

// GetPatchByID ...
func (*communicatorImpl) GetPatchByID() {
	return
}

// GetPatchesByProject ...
func (*communicatorImpl) GetPatchesByProject() {
	return
}

// SetPatchStatus ...
func (*communicatorImpl) SetPatchStatus() {
	return
}

// AbortPatch ...
func (*communicatorImpl) AbortPatch() {
	return
}

// RestartPatch ...
func (*communicatorImpl) RestartPatch() {
	return
}
