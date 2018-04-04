package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
)

// APIHost is the model to be returned by the API whenever hosts are fetched.
type APIHost struct {
	Id          APIString  `json:"host_id"`
	HostURL     APIString  `json:"host_url"`
	Distro      DistroInfo `json:"distro"`
	Provisioned bool       `json:"provisioned"`
	StartedBy   APIString  `json:"started_by"`
	Type        APIString  `json:"host_type"`
	User        APIString  `json:"user"`
	Status      APIString  `json:"status"`
	RunningTask taskInfo   `json:"running_task"`
	UserHost    bool       `json:"user_host"`
}

// HostPostRequest is a struct that holds the format of a POST request to /hosts
type HostPostRequest struct {
	DistroID string `json:"distro"`
	KeyName  string `json:"keyname"`
}

type DistroInfo struct {
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
	case host.Host, *host.Host:
		return apiHost.buildFromHostStruct(h)
	case *task.Task:
		apiHost.RunningTask = getTaskInfo(v)
	case task.Task:
		apiHost.RunningTask = getTaskInfo(&v)
	default:
		return fmt.Errorf("incorrect type when fetching converting host type")
	}
	return nil
}

func getTaskInfo(t *task.Task) taskInfo {
	return taskInfo{
		Id:           ToApiString(t.Id),
		Name:         ToApiString(t.DisplayName),
		DispatchTime: NewTime(t.DispatchTime),
		VersionId:    ToApiString(t.Version),
		BuildId:      ToApiString(t.BuildId),
	}
}

func (apiHost *APIHost) buildFromHostStruct(h interface{}) error {
	var v *host.Host
	switch h.(type) {
	case host.Host:
		t := h.(host.Host)
		v = &t
	case *host.Host:
		v = h.(*host.Host)
	default:
		return fmt.Errorf("incorrect type when fetching converting host type")
	}
	apiHost.Id = ToApiString(v.Id)
	apiHost.HostURL = ToApiString(v.Host)
	apiHost.Provisioned = v.Provisioned
	apiHost.StartedBy = ToApiString(v.StartedBy)
	apiHost.Type = ToApiString(v.InstanceType)
	apiHost.User = ToApiString(v.User)
	apiHost.Status = ToApiString(v.Status)
	apiHost.UserHost = v.UserHost

	di := DistroInfo{
		Id:       ToApiString(v.Distro.Id),
		Provider: ToApiString(v.Distro.Provider),
	}
	apiHost.Distro = di
	return nil
}

// ToService returns a service layer host using the data from the APIHost.
func (apiHost *APIHost) ToService() (interface{}, error) {
	h := host.Host{
		Id:           FromApiString(apiHost.Id),
		Provisioned:  apiHost.Provisioned,
		StartedBy:    FromApiString(apiHost.StartedBy),
		InstanceType: FromApiString(apiHost.Type),
		User:         FromApiString(apiHost.User),
		Status:       FromApiString(apiHost.Status),
	}
	return interface{}(h), nil
}

type APISpawnHostModify struct {
	Action   APIString `json:"action"`
	HostID   APIString `json:"host_id"`
	RDPPwd   APIString `json:"rdp_pwd"`
	AddHours APIString `json:"add_hours"`
}
