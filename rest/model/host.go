package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// APIHost is the model to be returned by the API whenever hosts are fetched.
type APIHost struct {
	Id                    *string     `json:"host_id"`
	HostURL               *string     `json:"host_url"`
	Tag                   *string     `json:"tag"`
	Distro                DistroInfo  `json:"distro"`
	Provisioned           bool        `json:"provisioned"`
	StartedBy             *string     `json:"started_by"`
	Provider              *string     `json:"host_type"`
	User                  *string     `json:"user"`
	Status                *string     `json:"status"`
	RunningTask           TaskInfo    `json:"running_task"`
	UserHost              bool        `json:"user_host"`
	NoExpiration          bool        `json:"no_expiration"`
	InstanceTags          []host.Tag  `json:"instance_tags"`
	InstanceType          *string     `json:"instance_type"`
	AvailabilityZone      *string     `json:"zone"`
	DisplayName           *string     `json:"display_name"`
	HomeVolumeID          *string     `json:"home_volume_id"`
	LastCommunicationTime time.Time   `json:"last_communication"`
	TotalIdleTime         APIDuration `json:"total_idle_time"`
	CreationTime          *time.Time  `json:"creation_time"`
	Expiration            *time.Time  `json:"expiration_time"`
	AttachedVolumeIDs     []string    `json:"attached_volume_ids"`
}

// HostRequestOptions is a struct that holds the format of a POST request to
// /hosts the yaml tags are used by hostCreate() when parsing the params from a
// file.
type HostRequestOptions struct {
	DistroID              string     `json:"distro" yaml:"distro"`
	TaskID                string     `json:"task" yaml:"task"`
	TaskSync              bool       `json:"task_sync" yaml:"task_sync"`
	Region                string     `json:"region" yaml:"region"`
	KeyName               string     `json:"keyname" yaml:"key"`
	UserData              string     `json:"userdata" yaml:"userdata_file"`
	SetupScript           string     `json:"setup_script" yaml:"setup_file"`
	UseProjectSetupScript bool       `json:"use_setup_script_path" yaml:"use_setup_script_path"`
	Tag                   string     `yaml:"tag"`
	InstanceTags          []host.Tag `json:"instance_tags" yaml:"instance_tags"`
	InstanceType          string     `json:"instance_type" yaml:"type"`
	NoExpiration          bool       `json:"no_expiration" yaml:"no-expire"`
	IsVirtualWorkstation  bool       `json:"is_virtual_workstation" yaml:"is_virtual_workstation"`
	IsCluster             bool       `json:"is_cluster" yaml:"is_cluster"`
	HomeVolumeSize        int        `json:"home_volume_size" yaml:"home_volume_size"`
	HomeVolumeID          string     `json:"home_volume_id" yaml:"home_volume_id"`
	Expiration            *time.Time `json:"expiration" yaml:"expiration"`
}

type DistroInfo struct {
	Id                   *string `json:"distro_id"`
	Provider             *string `json:"provider"`
	ImageId              *string `json:"image_id"`
	WorkDir              *string `json:"work_dir"`
	IsVirtualWorkstation bool    `json:"is_virtual_workstation"`
	User                 *string `json:"user"`
	IsWindows            bool    `json:"is_windows"`
	BootstrapMethod      *string `json:"bootstrap_method"`
}

type TaskInfo struct {
	Id           *string    `json:"task_id"`
	Name         *string    `json:"name"`
	DispatchTime *time.Time `json:"dispatch_time"`
	VersionId    *string    `json:"version_id"`
	BuildId      *string    `json:"build_id"`
	StartTime    *time.Time `json:"start_time"`
}

// BuildFromService converts from service level structs to an APIHost. If a task is given,
// we set that to the APIHost's running task.
func (apiHost *APIHost) BuildFromService(h *host.Host, t *task.Task) {
	apiHost.buildFromHostStruct(h)
	if t != nil {
		apiHost.RunningTask = TaskInfo{
			Id:           utility.ToStringPtr(t.Id),
			Name:         utility.ToStringPtr(t.DisplayName),
			DispatchTime: ToTimePtr(t.DispatchTime),
			VersionId:    utility.ToStringPtr(t.Version),
			BuildId:      utility.ToStringPtr(t.BuildId),
			StartTime:    ToTimePtr(t.StartTime),
		}
	}
}

func (apiHost *APIHost) buildFromHostStruct(h *host.Host) {
	if h == nil {
		return
	}
	apiHost.Id = utility.ToStringPtr(h.Id)
	apiHost.HostURL = utility.ToStringPtr(h.Host)
	apiHost.Tag = utility.ToStringPtr(h.Tag)
	apiHost.Provisioned = h.Provisioned
	apiHost.StartedBy = utility.ToStringPtr(h.StartedBy)
	apiHost.Provider = utility.ToStringPtr(h.Provider)
	apiHost.User = utility.ToStringPtr(h.User)
	apiHost.Status = utility.ToStringPtr(h.Status)
	apiHost.UserHost = h.UserHost
	apiHost.NoExpiration = h.NoExpiration
	apiHost.InstanceTags = h.InstanceTags
	apiHost.InstanceType = utility.ToStringPtr(h.InstanceType)
	apiHost.AvailabilityZone = utility.ToStringPtr(h.Zone)
	apiHost.DisplayName = utility.ToStringPtr(h.DisplayName)
	apiHost.HomeVolumeID = utility.ToStringPtr(h.HomeVolumeID)
	apiHost.TotalIdleTime = NewAPIDuration(h.TotalIdleTime)
	apiHost.CreationTime = ToTimePtr(h.CreationTime)
	apiHost.LastCommunicationTime = h.LastCommunicationTime
	apiHost.Expiration = ToTimePtr(h.ExpirationTime)
	attachedVolumeIds := []string{}
	for _, volAttachment := range h.Volumes {
		attachedVolumeIds = append(attachedVolumeIds, volAttachment.VolumeID)
	}
	apiHost.AttachedVolumeIDs = attachedVolumeIds
	imageId, err := h.Distro.GetImageID()
	if err != nil {
		// report error but do not fail function because of a bad imageId
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not get image ID",
			"host":    h.Id,
			"distro":  h.Distro.Id,
		}))
	}
	di := DistroInfo{
		Id:                   utility.ToStringPtr(h.Distro.Id),
		Provider:             utility.ToStringPtr(h.Distro.Provider),
		ImageId:              utility.ToStringPtr(imageId),
		WorkDir:              utility.ToStringPtr(h.Distro.WorkDir),
		IsVirtualWorkstation: h.Distro.IsVirtualWorkstation,
		User:                 utility.ToStringPtr(h.Distro.User),
		IsWindows:            h.Distro.IsWindows(),
		BootstrapMethod:      utility.ToStringPtr(h.Distro.BootstrapSettings.Method),
	}
	apiHost.Distro = di
}

// ToService returns a service layer host using the data from the APIHost.
func (apiHost *APIHost) ToService() host.Host {
	return host.Host{
		Id:                    utility.FromStringPtr(apiHost.Id),
		Tag:                   utility.FromStringPtr(apiHost.Tag),
		Provisioned:           apiHost.Provisioned,
		NoExpiration:          apiHost.NoExpiration,
		StartedBy:             utility.FromStringPtr(apiHost.StartedBy),
		Provider:              utility.FromStringPtr(apiHost.Provider),
		InstanceType:          utility.FromStringPtr(apiHost.InstanceType),
		User:                  utility.FromStringPtr(apiHost.User),
		Status:                utility.FromStringPtr(apiHost.Status),
		Zone:                  utility.FromStringPtr(apiHost.AvailabilityZone),
		DisplayName:           utility.FromStringPtr(apiHost.DisplayName),
		HomeVolumeID:          utility.FromStringPtr(apiHost.HomeVolumeID),
		LastCommunicationTime: apiHost.LastCommunicationTime,
	}
}

type APIVolume struct {
	ID               *string    `json:"volume_id"`
	DisplayName      *string    `json:"display_name"`
	CreatedBy        *string    `json:"created_by"`
	Type             *string    `json:"type"`
	AvailabilityZone *string    `json:"zone"`
	Size             int        `json:"size"`
	Expiration       *time.Time `json:"expiration"`
	DeviceName       *string    `json:"device_name"`
	HostID           *string    `json:"host_id"`
	NoExpiration     bool       `json:"no_expiration"`
	HomeVolume       bool       `json:"home_volume"`
	CreationTime     *time.Time `json:"creation_time"`
}

type VolumePostRequest struct {
	Type             string `json:"type"`
	Size             int    `json:"size"`
	AvailabilityZone string `json:"zone"`
}

type VolumeModifyOptions struct {
	NewName       string    `json:"new_name"`
	Size          int       `json:"size"`
	Expiration    time.Time `json:"expiration"`
	NoExpiration  bool      `json:"no_expiration"`
	HasExpiration bool      `json:"has_expiration"`
}

func (apiVolume *APIVolume) BuildFromService(v host.Volume) {
	apiVolume.ID = utility.ToStringPtr(v.ID)
	apiVolume.DisplayName = utility.ToStringPtr(v.DisplayName)
	apiVolume.CreatedBy = utility.ToStringPtr(v.CreatedBy)
	apiVolume.Type = utility.ToStringPtr(v.Type)
	apiVolume.AvailabilityZone = utility.ToStringPtr(v.AvailabilityZone)
	apiVolume.Size = v.Size
	apiVolume.HostID = utility.ToStringPtr(v.Host)
	apiVolume.Expiration = ToTimePtr(v.Expiration)
	apiVolume.NoExpiration = v.NoExpiration
	apiVolume.HomeVolume = v.HomeVolume
	apiVolume.CreationTime = ToTimePtr(v.CreationDate)
}

func (apiVolume *APIVolume) ToService() (host.Volume, error) {
	expiration, err := FromTimePtr(apiVolume.Expiration)
	if err != nil {
		return host.Volume{}, errors.Wrap(err, "getting expiration time")
	}

	return host.Volume{
		ID:               utility.FromStringPtr(apiVolume.ID),
		DisplayName:      utility.FromStringPtr(apiVolume.DisplayName),
		CreatedBy:        utility.FromStringPtr(apiVolume.CreatedBy),
		Type:             utility.FromStringPtr(apiVolume.Type),
		AvailabilityZone: utility.FromStringPtr(apiVolume.AvailabilityZone),
		Expiration:       expiration,
		Size:             apiVolume.Size,
		NoExpiration:     apiVolume.NoExpiration,
		HomeVolume:       apiVolume.HomeVolume,
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
	NewName      *string    `json:"new_name"`
}

type APIVolumeModify struct {
	Action     *string    `json:"action"`
	HostID     *string    `json:"host_id"`
	Expiration *time.Time `json:"expiration"`
	NewName    *string    `json:"new_name"`
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
	Status        string    `json:"status"`
	Region        string    `json:"region"`
	UserSpawned   bool      `json:"user_spawned"`
	Mine          bool      `json:"mine"`
}

type APIOffboardUserResults struct {
	TerminatedHosts   []string `json:"terminated_hosts"`
	TerminatedVolumes []string `json:"terminated_volumes"`
}

// APIHostProvisioningOptions represents the script to provision a host.
type APIHostProvisioningOptions struct {
	Content string `json:"content"`
}
