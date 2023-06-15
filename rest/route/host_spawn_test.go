package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostPostHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection))

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	env.EvergreenSettings.Spawnhost.SpawnHostsPerUser = 4
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(err)

	doc := birch.NewDocument(
		birch.EC.String("ami", "ami-123"),
		birch.EC.String("region", evergreen.DefaultEC2Region),
	)
	d := &distro.Distro{
		Id:                   "distro",
		SpawnAllowed:         true,
		Provider:             evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{doc},
	}
	require.NoError(d.Insert())
	assert.NoError(err)
	h := &hostPostHandler{
		env: env,
		options: &model.HostRequestOptions{
			TaskID:   "task",
			DistroID: "distro",
			KeyName:  "ssh-rsa YWJjZDEyMzQK",
		},
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	resp := h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h0 := resp.Data().(*model.APIHost)
	d0, err := distro.FindOneId("distro")
	assert.NoError(err)
	userdata, ok := d0.ProviderSettingsList[0].Lookup("user_data").StringValueOK()
	assert.False(ok)
	assert.Empty(userdata)
	assert.Empty(h0.InstanceTags)
	assert.Empty(h0.InstanceType)

	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	doc = birch.NewDocument(
		birch.EC.String("ami", "ami-123"),
		birch.EC.String("user_data", "#!/bin/bash\necho my script"),
		birch.EC.String("region", evergreen.DefaultEC2Region),
	)
	d.ProviderSettingsList = []*birch.Document{doc}
	assert.NoError(d.Update())

	h1 := resp.Data().(*model.APIHost)
	d1, err := distro.FindOneId("distro")
	assert.NoError(err)
	userdata, ok = d1.ProviderSettingsList[0].Lookup("user_data").StringValueOK()
	assert.True(ok)
	assert.Equal("#!/bin/bash\necho my script", userdata)
	assert.Empty(h1.InstanceTags)
	assert.Empty(h1.InstanceType)

	h.options.InstanceTags = []host.Tag{
		host.Tag{
			Key:           "ssh-rsa YWJjZDEyMzQK",
			Value:         "value",
			CanBeModified: true,
		},
	}
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h2 := resp.Data().(*model.APIHost)
	assert.Equal([]host.Tag{host.Tag{Key: "ssh-rsa YWJjZDEyMzQK", Value: "value", CanBeModified: true}}, h2.InstanceTags)
	assert.Empty(h2.InstanceType)

	d.Provider = evergreen.ProviderNameMock
	assert.NoError(d.Update())
	env.EvergreenSettings.Providers.AWS.AllowedInstanceTypes = append(env.EvergreenSettings.Providers.AWS.AllowedInstanceTypes, "test_instance_type")
	h.options.InstanceType = "test_instance_type"
	h.options.UserData = ""
	resp = h.Run(ctx)
	require.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h3 := resp.Data().(*model.APIHost)
	assert.Equal("test_instance_type", *h3.InstanceType)
}

func TestHostStopHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, event.SubscriptionsCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, event.SubscriptionsCollection))
	}()
	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &hostStopHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	hosts := []host.Host{
		host.Host{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
		host.Host{
			Id:     "host-stopping",
			Status: evergreen.HostStopping,
		},
		host.Host{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
		host.Host{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert())
	}

	h.hostID = hosts[0].Id
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = hosts[1].Id
	resp = h.Run(ctx)
	require.NotZero(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	h.hostID = hosts[2].Id
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = hosts[3].Id
	h.subscriptionType = event.SlackSubscriberType
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	subscriptions, err := data.GetSubscriptions("user", event.OwnerTypePerson)
	assert.NoError(t, err)
	assert.Len(t, subscriptions, 1)
}

func TestHostStartHandler(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.SubscriptionsCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.SubscriptionsCollection))
	}()
	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &hostStartHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	hosts := []host.Host{
		host.Host{
			Id:       "host-running",
			Status:   evergreen.HostRunning,
			UserHost: true,
		},
		host.Host{
			Id:       "host-stopped",
			UserHost: true,
			Status:   evergreen.HostStopped,
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert())
	}

	h.hostID = "host-running"
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = "host-stopped"
	h.subscriptionType = event.SlackSubscriberType
	resp = h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	subscriptions, err := data.GetSubscriptions("user", event.OwnerTypePerson)
	assert.NoError(t, err)
	assert.Len(t, subscriptions, 1)
}

func TestCreateVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &createVolumeHandler{
		env:      testutil.NewEnvironment(ctx, t),
		provider: evergreen.ProviderNameMock,
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	v := host.Volume{ID: "volume1", Size: 15, CreatedBy: "user"}
	assert.NoError(t, v.Insert())
	v = host.Volume{ID: "volume2", Size: 35, CreatedBy: "user"}
	assert.NoError(t, v.Insert())
	v = host.Volume{ID: "not-relevant", Size: 400, CreatedBy: "someone-else"}
	assert.NoError(t, v.Insert())

	h.env.Settings().Providers.AWS.MaxVolumeSizePerUser = 100
	h.env.Settings().Providers.AWS.Subnets = []evergreen.Subnet{
		{AZ: "us-east-1a", SubnetID: "123"},
	}
	v = host.Volume{ID: "volume-new", Size: 80, CreatedBy: "user"}
	h.volume = &v

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	v.Size = 50
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestDeleteVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &deleteVolumeHandler{
		env:      testutil.NewEnvironment(ctx, t),
		provider: evergreen.ProviderNameMock,
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	volumes := []host.Volume{
		{
			ID:               "my-volume",
			CreatedBy:        "user",
			AvailabilityZone: "us-east-1a",
		},
	}
	hosts := []host.Host{
		{
			Id:        "my-host",
			UserHost:  true,
			StartedBy: "user",
			Status:    evergreen.HostRunning,
			Volumes: []host.VolumeAttachment{
				{
					VolumeID:   "my-volume",
					DeviceName: "my-device",
				},
			},
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert())
	}
	for _, volumeToAdd := range volumes {
		assert.NoError(t, volumeToAdd.Insert())
	}
	h.VolumeID = "my-volume"
	resp := h.Run(ctx)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

func TestAttachVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &attachVolumeHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	hosts := []host.Host{
		host.Host{
			Id:        "my-host",
			Status:    evergreen.HostRunning,
			StartedBy: "user",
			Zone:      "us-east-1c",
		},
		host.Host{
			Id: "different-host",
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert())
	}

	// no volume
	v := &host.VolumeAttachment{DeviceName: "my-device"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest(http.MethodGet, "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.Error(t, h.Parse(ctx, r))

	// wrong availability zone
	v.VolumeID = "my-volume"
	volume := host.Volume{
		ID: v.VolumeID,
	}
	assert.NoError(t, volume.Insert())

	jsonBody, err = json.Marshal(v)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest(http.MethodGet, "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))

	require.NotNil(t, h.attachment)
	assert.Equal(t, h.attachment.VolumeID, "my-volume")
	assert.Equal(t, h.attachment.DeviceName, "my-device")

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

func TestDetachVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &detachVolumeHandler{
		env: testutil.NewEnvironment(ctx, t),
	}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	hosts := []host.Host{
		host.Host{
			Id:        "my-host",
			StartedBy: "user",
			Status:    evergreen.HostRunning,
			Volumes: []host.VolumeAttachment{
				{
					VolumeID:   "my-volume",
					DeviceName: "my-device",
				},
			},
		},
	}
	for _, hostToAdd := range hosts {
		assert.NoError(t, hostToAdd.Insert())
	}

	v := host.VolumeAttachment{VolumeID: "not-a-volume"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest(http.MethodGet, "/hosts/my-host/detach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.Status())
}

func TestModifyVolumeHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &modifyVolumeHandler{
		env:  testutil.NewEnvironment(ctx, t),
		opts: &model.VolumeModifyOptions{},
	}
	h.env.Settings().Providers.AWS.MaxVolumeSizePerUser = 200
	h.env.Settings().Spawnhost.UnexpirableVolumesPerUser = 1
	volume := host.Volume{
		ID:               "volume1",
		CreatedBy:        "user",
		Size:             64,
		AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
	}
	assert.NoError(t, volume.Insert())

	// parse request
	opts := &model.VolumeModifyOptions{Size: 20, NewName: "my-favorite-volume"}
	jsonBody, err := json.Marshal(opts)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)
	r, err := http.NewRequest("", "", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"volume_id": "volume1"})
	assert.NoError(t, h.Parse(context.Background(), r))
	assert.Equal(t, "volume1", h.volumeID)
	assert.EqualValues(t, 20, h.opts.Size)
	assert.Equal(t, "my-favorite-volume", h.opts.NewName)

	h.provider = evergreen.ProviderNameMock
	h.opts = &model.VolumeModifyOptions{}
	// another user
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "different-user"})
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.Status())

	// volume's owner
	ctx = gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// resize
	h.opts.Size = 200
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// resize, exceeding max size
	h.opts = &model.VolumeModifyOptions{Size: 500}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	// set expiration
	h.opts = &model.VolumeModifyOptions{Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// no expiration
	h.opts = &model.VolumeModifyOptions{NoExpiration: true}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	// has expiration
	h.opts = &model.VolumeModifyOptions{HasExpiration: true}
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestGetVolumesHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	h := &getVolumesHandler{}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	h1 := host.Host{
		Id:        "has-a-volume",
		StartedBy: "user",
		Volumes: []host.VolumeAttachment{
			{VolumeID: "volume1", DeviceName: "/dev/sdf4"},
		},
	}

	volumesToAdd := []host.Volume{
		{
			ID:               "volume1",
			Host:             "has-a-volume",
			CreatedBy:        "user",
			Type:             evergreen.DefaultEBSType,
			Size:             64,
			AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
		},
		{
			ID:               "volume2",
			CreatedBy:        "user",
			Type:             evergreen.DefaultEBSType,
			Size:             36,
			AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
		},
		{
			ID:        "volume3",
			CreatedBy: "different-user",
		},
	}
	for _, volumeToAdd := range volumesToAdd {
		assert.NoError(t, volumeToAdd.Insert())
	}
	assert.NoError(t, h1.Insert())
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	volumes, ok := resp.Data().([]model.APIVolume)
	assert.True(t, ok)
	require.Len(t, volumes, 2)

	for _, v := range volumes {
		assert.Equal(t, "user", utility.FromStringPtr(v.CreatedBy))
		assert.Equal(t, evergreen.DefaultEBSType, utility.FromStringPtr(v.Type))
		assert.Equal(t, evergreen.DefaultEBSAvailabilityZone, utility.FromStringPtr(v.AvailabilityZone))
		if utility.FromStringPtr(v.ID) == "volume1" {
			assert.Equal(t, h1.Id, utility.FromStringPtr(v.HostID))
			assert.Equal(t, h1.Volumes[0].DeviceName, utility.FromStringPtr(v.DeviceName))
			assert.Equal(t, v.Size, 64)
		} else {
			assert.Empty(t, utility.FromStringPtr(v.HostID))
			assert.Empty(t, utility.FromStringPtr(v.DeviceName))
			assert.Equal(t, v.Size, 36)
		}
	}
}

func TestGetVolumeByIDHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection))
	h := &getVolumeByIDHandler{}
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	h1 := host.Host{
		Id:        "has-a-volume",
		StartedBy: "user",
		Volumes: []host.VolumeAttachment{
			{VolumeID: "volume1", DeviceName: "/dev/sdf4"},
		},
	}

	volume := host.Volume{
		ID:               "volume1",
		Host:             "has-a-volume",
		CreatedBy:        "user",
		Type:             evergreen.DefaultEBSType,
		Size:             64,
		AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
	}
	assert.NoError(t, volume.Insert())
	assert.NoError(t, h1.Insert())
	r, err := http.NewRequest(http.MethodGet, "/volumes/volume1", nil)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"volume_id": "volume1"})
	assert.NoError(t, h.Parse(ctx, r))

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	v, ok := resp.Data().(*model.APIVolume)
	assert.True(t, ok)
	require.NotNil(t, v)
	assert.Equal(t, "user", utility.FromStringPtr(v.CreatedBy))
	assert.Equal(t, evergreen.DefaultEBSType, utility.FromStringPtr(v.Type))
	assert.Equal(t, evergreen.DefaultEBSAvailabilityZone, utility.FromStringPtr(v.AvailabilityZone))
	assert.Equal(t, h1.Id, utility.FromStringPtr(v.HostID))
	assert.Equal(t, h1.Volumes[0].DeviceName, utility.FromStringPtr(v.DeviceName))
	assert.Equal(t, v.Size, 64)
}

func TestMakeSpawnHostSubscription(t *testing.T) {
	user := &user.DBUser{
		EmailAddress: "evergreen@mongodb.com",
		Settings: user.UserSettings{
			SlackUsername: "mci",
		},
	}
	_, err := makeSpawnHostSubscription("id", "non-existent", user)
	assert.Error(t, err)

	sub, err := makeSpawnHostSubscription("id", event.SlackSubscriberType, user)
	assert.NoError(t, err)
	assert.Equal(t, event.ResourceTypeHost, utility.FromStringPtr(sub.ResourceType))
	assert.Len(t, sub.Selectors, 1)
	assert.Equal(t, event.SlackSubscriberType, utility.FromStringPtr(sub.Subscriber.Type))
	assert.Equal(t, "@mci", sub.Subscriber.Target)
}
