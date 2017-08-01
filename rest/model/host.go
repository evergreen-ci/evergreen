package model

import (
	"errors"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
)

// APIHost is the model to be returned by the API whenever hosts are fetched.
type APIHost struct {
	Id          APIString  `json:"host_id"`
	Distro      distroInfo `json:"distro"`
	Provisioned bool       `json:"provisioned"`
	StartedBy   APIString  `json:"started_by"`
	Type        APIString  `json:"host_type"`
	User        APIString  `json:"user"`
	Status      APIString  `json:"status"`
	RunningTask taskInfo   `json:"running_task"`
}

// HostPostRequest is a struct that holds the format of a POST request to /hosts
type HostPostRequest struct {
	DistroID string `json:"distro"`
	KeyName  string `json:"keyname"`
}

// SpawnHost is many fields from the host.Host object that is returned in the response
// to POST /hosts
type SpawnHost struct {
	HostID         APIString `json:"host_id"`
	DistroID       APIString `json:"distro_id"`
	Type           APIString `json:"host_type"`
	ExpirationTime APITime   `json:"expiration_time"`
	CreationTime   APITime   `json:"creation_time"`
	Status         APIString `json:"status"`
	StartedBy      APIString `json:"started_by"`
	Tag            APIString `json:"tag"`
	Project        APIString `json:"project"`
	Zone           APIString `json:"zone"`
	UserHost       bool      `json:"user_host"`
	Provisioned    bool      `json:"provisioned"`
}

type distroInfo struct {
	Id       APIString `json:"distro_id"`
	Provider APIString `json:"provider"`
}

type taskInfo struct {
	Id           APIString `json:"task_id"`
	Name         APIString `json:"name"`
	DispatchTime APITime   `json:"dispatch_time"`
	VersionId    APIString `json:"version_id"`
	BuildId      APIString `json:"build_id"`
}

// BuildFromService converts from service level structs to an APIHost. It can
// be called multiple times with different data types, a service layer host and
// a service layer task, which are each loaded into the data structure.
func (apiHost *APIHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		apiHost.Id = APIString(v.Id)
		apiHost.Provisioned = v.Provisioned
		apiHost.StartedBy = APIString(v.StartedBy)
		apiHost.Type = APIString(v.InstanceType)
		apiHost.User = APIString(v.UserData)
		apiHost.Status = APIString(v.Status)

		di := distroInfo{
			Id:       APIString(v.Distro.Id),
			Provider: APIString(v.Distro.Provider),
		}
		apiHost.Distro = di
	case task.Task:
		rt := taskInfo{
			Id:           APIString(v.Id),
			Name:         APIString(v.DisplayName),
			DispatchTime: NewTime(v.DispatchTime),
			VersionId:    APIString(v.Version),
			BuildId:      APIString(v.BuildId),
		}
		apiHost.RunningTask = rt
	default:
		return fmt.Errorf("incorrect type when fetching converting host type")
	}
	return nil
}

// ToService returns a service layer host using the data from the APIHost.
func (apiHost *APIHost) ToService() (interface{}, error) {
	h := host.Host{
		Id:           string(apiHost.Id),
		Provisioned:  apiHost.Provisioned,
		StartedBy:    string(apiHost.StartedBy),
		InstanceType: string(apiHost.Type),
		UserData:     string(apiHost.User),
		Status:       string(apiHost.Status),
	}
	return interface{}(h), nil
}

// BuildFromService takes the intent host passed in by the service and creates a spawnHost struct
func (spawnHost *SpawnHost) BuildFromService(h interface{}) error {
	host := h.(*host.Host)

	spawnHost.HostID = APIString(host.Id)
	spawnHost.DistroID = APIString(host.Distro.Id)
	spawnHost.Type = APIString(host.Provider)
	spawnHost.ExpirationTime = APITime(host.ExpirationTime)
	spawnHost.CreationTime = APITime(host.CreationTime)
	spawnHost.Status = APIString(host.Status)
	spawnHost.StartedBy = APIString(host.StartedBy)
	spawnHost.Tag = APIString(host.Tag)
	spawnHost.Project = APIString(host.Project)
	spawnHost.Zone = APIString(host.Zone)
	spawnHost.UserHost = host.UserHost
	spawnHost.Provisioned = host.Provisioned

	return nil
}

// ToService extracts the intent host part of a spawn host
func (spawnHost *SpawnHost) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for /hosts")
}
