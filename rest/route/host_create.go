package route

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type hostCreateHandler struct {
	taskID     string
	createHost apimodels.CreateHost

	sc data.Connector
}

func makeHostCreateRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &hostCreateHandler{sc: sc}
}

func (h *hostCreateHandler) Factory() gimlet.RouteHandler { return &hostCreateHandler{sc: h.sc} }

func (h *hostCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}
	if _, code, err := dbModel.ValidateHost("", r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "host is invalid",
		}
	}
	if err := util.ReadJSONInto(r.Body, h.createHost); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	return h.createHost.Validate()
}

func (h *hostCreateHandler) Run(ctx context.Context) gimlet.Responder {
	hosts := []host.Host{}
	for i := 0; i < h.createHost.NumHosts; i++ {
		intentHost, err := h.makeIntentHost()
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		hosts = append(hosts, *intentHost)
	}

	if err := host.InsertMany(hosts); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (h *hostCreateHandler) makeIntentHost() (*host.Host, error) {
	provider := evergreen.ProviderNameEc2OnDemand
	if h.createHost.Spot {
		provider = evergreen.ProviderNameEc2Spot
	}

	// get distro if it is set
	d := distro.Distro{}
	ec2Settings := cloud.EC2ProviderSettings{}
	var err error
	if distroID := h.createHost.Distro; distroID != "" {
		d, err = distro.FindOne(distro.ById(distroID))
		if err != nil {
			return nil, errors.Wrap(err, "problem finding distro")
		}
		if err := mapstructure.Decode(d.ProviderSettings, &ec2Settings); err != nil {
			return nil, errors.Wrap(err, "problem unmarshaling provider settings")
		}
	}

	// set provider settings
	if h.createHost.AMI != "" {
		ec2Settings.AMI = h.createHost.AMI
	}
	if h.createHost.AWSKeyID != "" {
		ec2Settings.AWSKeyID = h.createHost.AWSKeyID
		ec2Settings.AWSSecret = h.createHost.AWSSecret
	}

	for _, mount := range h.createHost.EBSDevices {
		ec2Settings.MountPoints = append(ec2Settings.MountPoints, cloud.MountPoint{
			DeviceName: mount.DeviceName,
			Size:       int64(mount.SizeGiB),
			Iops:       int64(mount.IOPS),
			SnapshotID: mount.SnapshotID,
		})
	}
	if h.createHost.InstanceType != "" {
		ec2Settings.InstanceType = h.createHost.InstanceType
	}
	if h.createHost.KeyName != "" {
		ec2Settings.KeyName = h.createHost.KeyName
	}
	if h.createHost.Region != "" {
		ec2Settings.Region = h.createHost.Region
	}
	if len(h.createHost.SecurityGroups) > 0 {
		ec2Settings.SecurityGroupIDs = h.createHost.SecurityGroups
	}
	if h.createHost.Subnet != "" {
		ec2Settings.SubnetId = h.createHost.Subnet
	}
	if h.createHost.UserdataCommand != "" {
		ec2Settings.UserData = h.createHost.UserdataCommand
	}
	if h.createHost.VPC != "" {
		ec2Settings.VpcName = h.createHost.VPC
	}
	if err := mapstructure.Decode(ec2Settings, &d.ProviderSettings); err != nil {
		return nil, errors.Wrap(err, "error marshaling provider settings")
	}

	// scope and teardown options
	options := cloud.HostOptions{}
	options.UserName = h.taskID
	if h.createHost.Scope == "build" {
		t, err := task.FindOneId(h.taskID)
		if err != nil {
			return nil, errors.Wrap(err, "could not find task")
		}
		if t == nil {
			return nil, errors.New("no task returned")
		}
		options.SpawnOptions.BuildID = t.BuildId
	}
	if h.createHost.Scope == "task" {
		options.SpawnOptions.TaskID = h.taskID
	}
	options.SpawnOptions.TimeoutTeardown = time.Now().Add(time.Duration(h.createHost.TeardownTimeoutSecs) * time.Second)
	options.SpawnOptions.TimeoutSetup = time.Now().Add(time.Duration(h.createHost.SetupTimeoutSecs) * time.Second)
	options.SpawnOptions.Retries = h.createHost.Retries
	options.SpawnOptions.SpawnedByTask = true

	return cloud.NewIntent(d, d.GenerateName(), provider, options), nil
}

type hostListHandler struct {
	taskID string

	sc data.Connector
}

func makeHostListRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &hostListHandler{sc: sc}
}

func (h *hostListHandler) Factory() gimlet.RouteHandler { return &hostListHandler{sc: h.sc} }

func (h *hostListHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}

	return nil
}

func (h *hostListHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := h.sc.ListHostsForTask(h.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	catcher := grip.NewBasicCatcher()
	results := make([]model.Model, len(hosts))
	for i := range hosts {
		createHost := model.CreateHost{}
		if err := createHost.BuildFromService(&hosts[i]); err != nil {
			catcher.Add(errors.Wrap(err, "error building api host from service"))
		}
		results[i] = &createHost
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(catcher.Resolve())
	}
	return gimlet.NewJSONResponse(results)
}

// HostDockerfile route returns Dockerfile in text response.
// Add the following to user.data for parent distros:
// curl https://evergreen.mongodb.com/rest/v2/hosts/dockerfile > /root/Dockerfile

type hostDockerfileHandler struct {
}

func makeHostDockerfileRouteManager() gimlet.RouteHandler {
	return &hostDockerfileHandler{}
}

func (h *hostDockerfileHandler) Factory() gimlet.RouteHandler { return &hostDockerfileHandler{} }

func (h *hostDockerfileHandler) Parse(ctx context.Context, r *http.Request) error { return nil }

func (h *hostDockerfileHandler) Run(ctx context.Context) gimlet.Responder {
	parts := []string{
		"ARG BASE_IMAGE",
		"FROM $BASE_IMAGE",
		"ARG EXECUTABLE_SUB_PATH",
		"ARG URL",
		"ADD ${URL}/clients/${EXECUTABLE_SUB_PATH} /root/",
	}
	file := strings.Join(parts, "\n")

	return gimlet.NewTextResponse(file)
}
