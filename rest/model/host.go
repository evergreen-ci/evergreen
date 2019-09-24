package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APIHost is the model to be returned by the API whenever hosts are fetched.
type APIHost struct {
	Id           APIString  `json:"host_id"`
	HostURL      APIString  `json:"host_url"`
	Distro       DistroInfo `json:"distro"`
	Provisioned  bool       `json:"provisioned"`
	StartedBy    APIString  `json:"started_by"`
	Type         APIString  `json:"host_type"`
	User         APIString  `json:"user"`
	Status       APIString  `json:"status"`
	RunningTask  taskInfo   `json:"running_task"`
	UserHost     bool       `json:"user_host"`
	InstanceTags []host.Tag `json:"instance_tags"`
}

// HostPostRequest is a struct that holds the format of a POST request to /hosts
type HostRequestOptions struct {
	DistroID     string     `json:"distro"`
	TaskID       string     `json:"task"`
	KeyName      string     `json:"keyname"`
	UserData     string     `json:"userdata"`
	InstanceTags []host.Tag `json:"instance_tags"`
}

type DistroInfo struct {
	Id       APIString `json:"distro_id"`
	Provider APIString `json:"provider"`
	ImageId  APIString `json:"image_id"`
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
		return errors.New("incorrect type when fetching converting host type")
	}
	return nil
}

func getTaskInfo(t *task.Task) taskInfo {
	return taskInfo{
		Id:           ToAPIString(t.Id),
		Name:         ToAPIString(t.DisplayName),
		DispatchTime: NewTime(t.DispatchTime),
		VersionId:    ToAPIString(t.Version),
		BuildId:      ToAPIString(t.BuildId),
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
		return errors.New("incorrect type when fetching converting host type")
	}
	apiHost.Id = ToAPIString(v.Id)
	apiHost.HostURL = ToAPIString(v.Host)
	apiHost.Provisioned = v.Provisioned
	apiHost.StartedBy = ToAPIString(v.StartedBy)
	apiHost.Type = ToAPIString(v.InstanceType)
	apiHost.User = ToAPIString(v.User)
	apiHost.Status = ToAPIString(v.Status)
	apiHost.UserHost = v.UserHost
	apiHost.InstanceTags = v.InstanceTags

	imageId, err := v.Distro.GetImageID()
	if err != nil {
		return errors.Wrap(err, "problem getting image ID")
	}
	di := DistroInfo{
		Id:       ToAPIString(v.Distro.Id),
		Provider: ToAPIString(v.Distro.Provider),
		ImageId:  ToAPIString(imageId),
	}
	apiHost.Distro = di
	return nil
}

// ToService returns a service layer host using the data from the APIHost.
func (apiHost *APIHost) ToService() (interface{}, error) {
	h := host.Host{
		Id:           FromAPIString(apiHost.Id),
		Provisioned:  apiHost.Provisioned,
		StartedBy:    FromAPIString(apiHost.StartedBy),
		InstanceType: FromAPIString(apiHost.Type),
		User:         FromAPIString(apiHost.User),
		Status:       FromAPIString(apiHost.Status),
	}
	return interface{}(h), nil
}

type APISpawnHostModify struct {
	Action     APIString `json:"action"`
	HostID     APIString `json:"host_id"`
	RDPPwd     APIString `json:"rdp_pwd"`
	AddHours   APIString `json:"add_hours"`
	Expiration time.Time `json:"expiration"`
}
