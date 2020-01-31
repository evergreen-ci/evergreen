package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// APIHost is the model to be returned by the API whenever hosts are fetched.
type APIHost struct {
	Id               *string    `json:"host_id"`
	HostURL          *string    `json:"host_url"`
	Distro           DistroInfo `json:"distro"`
	Provisioned      bool       `json:"provisioned"`
	StartedBy        *string    `json:"started_by"`
	Type             *string    `json:"host_type"`
	User             *string    `json:"user"`
	Status           *string    `json:"status"`
	RunningTask      taskInfo   `json:"running_task"`
	UserHost         bool       `json:"user_host"`
	InstanceTags     []host.Tag `json:"instance_tags"`
	InstanceType     *string    `json:"instance_type"`
	AvailabilityZone *string    `json:"zone"`
}

// HostPostRequest is a struct that holds the format of a POST request to /hosts
type HostRequestOptions struct {
	DistroID     string     `json:"distro"`
	TaskID       string     `json:"task"`
	KeyName      string     `json:"keyname"`
	UserData     string     `json:"userdata"`
	InstanceTags []host.Tag `json:"instance_tags"`
	InstanceType string     `json:"instance_type"`
	NoExpiration bool       `json:"no_expiration"`
}

type DistroInfo struct {
	Id       *string `json:"distro_id"`
	Provider *string `json:"provider"`
	ImageId  *string `json:"image_id"`
}

type taskInfo struct {
	Id           *string    `json:"task_id"`
	Name         *string    `json:"name"`
	DispatchTime *time.Time `json:"dispatch_time"`
	VersionId    *string    `json:"version_id"`
	BuildId      *string    `json:"build_id"`
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
		Id:           ToStringPtr(t.Id),
		Name:         ToStringPtr(t.DisplayName),
		DispatchTime: ToTimePtr(t.DispatchTime),
		VersionId:    ToStringPtr(t.Version),
		BuildId:      ToStringPtr(t.BuildId),
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
	apiHost.Id = ToStringPtr(v.Id)
	apiHost.HostURL = ToStringPtr(v.Host)
	apiHost.Provisioned = v.Provisioned
	apiHost.StartedBy = ToStringPtr(v.StartedBy)
	apiHost.Type = ToStringPtr(v.InstanceType)
	apiHost.User = ToStringPtr(v.User)
	apiHost.Status = ToStringPtr(v.Status)
	apiHost.UserHost = v.UserHost
	apiHost.InstanceTags = v.InstanceTags
	apiHost.InstanceType = ToStringPtr(v.InstanceType)
	apiHost.AvailabilityZone = ToStringPtr(v.Zone)

	imageId, err := v.Distro.GetImageID()
	if err != nil {
		return errors.Wrap(err, "problem getting image ID")
	}
	di := DistroInfo{
		Id:       ToStringPtr(v.Distro.Id),
		Provider: ToStringPtr(v.Distro.Provider),
		ImageId:  ToStringPtr(imageId),
	}
	apiHost.Distro = di
	return nil
}

// ToService returns a service layer host using the data from the APIHost.
func (apiHost *APIHost) ToService() (interface{}, error) {
	h := host.Host{
		Id:           FromStringPtr(apiHost.Id),
		Provisioned:  apiHost.Provisioned,
		StartedBy:    FromStringPtr(apiHost.StartedBy),
		InstanceType: FromStringPtr(apiHost.Type),
		User:         FromStringPtr(apiHost.User),
		Status:       FromStringPtr(apiHost.Status),
		Zone:         FromStringPtr(apiHost.AvailabilityZone),
	}
	return interface{}(h), nil
}

type APIVolume struct {
	ID               *string `json:"volume_id"`
	CreatedBy        *string `json:"created_by"`
	Type             *string `json:"type"`
	AvailabilityZone *string `json:"zone"`
	Size             int     `json:"size"`

	DeviceName *string `json:"device_name"`
	HostID     *string `json:"host_id"`
}

type VolumePostRequest struct {
	Type             string `json:"type"`
	Size             int    `json:"size"`
	AvailabilityZone string `json:"zone"`
}

func (apiVolume *APIVolume) BuildFromService(volume interface{}) error {
	switch volume.(type) {
	case host.Volume, *host.Volume:
		return apiVolume.buildFromVolumeStruct(volume)
	default:
		return errors.Errorf("%T is not a supported type", volume)
	}
}

func (apiVolume *APIVolume) buildFromVolumeStruct(volume interface{}) error {
	var v *host.Volume
	switch volume.(type) {
	case host.Volume:
		t := volume.(host.Volume)
		v = &t
	case *host.Volume:
		v = volume.(*host.Volume)
	default:
		return errors.New("incorrect type when converting volume type")
	}
	apiVolume.ID = ToStringPtr(v.ID)
	apiVolume.CreatedBy = ToStringPtr(v.CreatedBy)
	apiVolume.Type = ToStringPtr(v.Type)
	apiVolume.AvailabilityZone = ToStringPtr(v.AvailabilityZone)
	apiVolume.Size = v.Size
	return nil
}

func (apiVolume *APIVolume) ToService() (interface{}, error) {
	return host.Volume{
		ID:               FromStringPtr(apiVolume.ID),
		CreatedBy:        FromStringPtr(apiVolume.CreatedBy),
		Type:             FromStringPtr(apiVolume.Type),
		AvailabilityZone: FromStringPtr(apiVolume.AvailabilityZone),
		Size:             apiVolume.Size,
	}, nil
}

type APISpawnHostModify struct {
	Action       *string    `json:"action"`
	HostID       *string    `json:"host_id"`
	VolumeID     *string    `json:"volume_id"`
	RDPPwd       *string    `json:"rdp_pwd"`
	AddHours     *string    `json:"add_hours"`
	Expiration   *time.Time `json:"expiration"`
	InstanceType *string    `json:"instance_type"`
	AddTags      []*string  `json:"tags_to_add"`
	DeleteTags   []*string  `json:"tags_to_delete"`
}

type APIHostScript struct {
	Hosts  []string `json:"hosts"`
	Script string   `json:"script"`
}

type APIHostProcess struct {
	HostID   string `json:"host_id"`
	ProcID   string `json:"proc_id"`
	Complete bool   `json:"complete"`
	Output   string `json:"output"`
}

type APIHostParams struct {
	CreatedBefore time.Time `json:"created_before"`
	CreatedAfter  time.Time `json:"created_after"`
	Distro        string    `json:"distro"`
	UserSpawned   bool      `json:"user_spawned"`
	Status        string    `json:"status"`
	Mine          bool      `json:"mine"`
}
