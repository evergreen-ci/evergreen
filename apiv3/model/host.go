package model

import (
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
			DispatchTime: APITime(v.DispatchTime),
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
