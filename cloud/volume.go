package cloud

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func CreateVolume(ctx context.Context, env evergreen.Environment, volume *host.Volume, provider string) (*host.Volume, error) {
	mgrOpts := ManagerOpts{
		Provider: provider,
		Region:   AztoRegion(volume.AvailabilityZone),
	}
	mgr, err := GetManager(ctx, env, mgrOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "getting cloud manager")
	}

	if volume, err = mgr.CreateVolume(ctx, volume); err != nil {
		return nil, errors.Wrapf(err, "creating volume")
	}
	return volume, nil
}

func GetEC2ManagerForVolume(ctx context.Context, vol *host.Volume) (Manager, error) {
	provider := evergreen.ProviderNameEc2OnDemand
	// WARNING: We unfortunately have to hard-code variables for E2E testing.
	// Note that this should be avoided when possible, but is necessary in this case.
	if os.Getenv(evergreen.SettingsOverride) != "" {
		// Use the mock manager during integration tests
		provider = evergreen.ProviderNameMock
		// Set a host that will be utilized during Spruce e2e tests in spawn/volume.ts.
		// A host is required to be set in order to unmount or delete a volume.
		mockState := GetMockProvider()
		mockHostID := "7f909d47566126bd39a05c1a5bd5d111c2e68de3830a8be414c18c231a47f4fc"
		mockState.Set(mockHostID, MockInstance{})
	}
	mgrOpts := ManagerOpts{
		Provider: provider,
		Region:   AztoRegion(vol.AvailabilityZone),
	}
	env := evergreen.GetEnvironment()
	mgr, err := GetManager(ctx, env, mgrOpts)
	return mgr, errors.Wrapf(err, "getting cloud manager for volume '%s'", vol.ID)
}

func DeleteVolume(ctx context.Context, volumeId string) (int, error) {
	if volumeId == "" {
		return http.StatusBadRequest, errors.New("must specify volume ID")
	}

	vol, err := host.FindVolumeByID(ctx, volumeId)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "getting volume '%s'", volumeId)
	}
	if vol == nil {
		return http.StatusNotFound, errors.Errorf("volume '%s' not found", volumeId)
	}
	if vol.Host != "" {
		statusCode, detachErr := DetachVolume(ctx, volumeId)
		if detachErr != nil {
			return statusCode, detachErr
		}
	}
	mgr, err := GetEC2ManagerForVolume(ctx, vol)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	err = mgr.DeleteVolume(ctx, vol)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "deleting volume '%s'", vol.ID)
	}
	return http.StatusOK, nil
}

func AttachVolume(ctx context.Context, volumeId string, hostId string) (int, error) {
	if volumeId == "" {
		return http.StatusBadRequest, errors.New("must specify volume ID")
	}
	vol, err := host.FindVolumeByID(ctx, volumeId)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "getting volume '%s'", volumeId)
	}
	if vol == nil {
		return http.StatusNotFound, errors.Errorf("volume '%s' not found", volumeId)
	}
	mgr, err := GetEC2ManagerForVolume(ctx, vol)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if hostId == "" {
		return http.StatusBadRequest, errors.New("must specify host ID")
	}
	h, err := host.FindOneId(ctx, hostId)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "getting host '%s'", vol.Host)
	}
	if h == nil {
		return http.StatusNotFound, errors.Errorf("host '%s' not found", hostId)
	}

	if vol.AvailabilityZone != h.Zone {
		return http.StatusBadRequest, errors.New("host and volume must have same availability zone")
	}
	if err = mgr.AttachVolume(ctx, h, &host.VolumeAttachment{VolumeID: vol.ID}); err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "attaching volume '%s' to host '%s'", vol.ID, h.Id)
	}
	return http.StatusOK, nil
}

func DetachVolume(ctx context.Context, volumeId string) (int, error) {
	if volumeId == "" {
		return http.StatusBadRequest, errors.New("must specify volume ID")
	}
	vol, err := host.FindVolumeByID(ctx, volumeId)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "getting volume '%s'", volumeId)
	}
	if vol == nil {
		return http.StatusNotFound, errors.Errorf("volume '%s' does not exist", volumeId)
	}
	mgr, err := GetEC2ManagerForVolume(ctx, vol)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if vol.Host == "" {
		return http.StatusBadRequest, errors.Errorf("volume '%s' is not attached", vol.ID)
	}
	h, err := host.FindOneId(ctx, vol.Host)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "can't get host '%s' for volume '%s'", vol.Host, vol.ID)
	}
	if h == nil {
		if err = host.UnsetVolumeHost(ctx, vol.ID); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": fmt.Sprintf("can't clear host '%s' from volume '%s'", vol.Host, vol.ID),
				"route":   "graphql/util",
				"action":  "DetachVolume",
			}))
		}
		return http.StatusInternalServerError, errors.Errorf("host '%s' for volume '%s' not found", vol.Host, vol.ID)
	}

	if err := mgr.DetachVolume(ctx, h, vol.ID); err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "detaching volume '%s'", vol.ID)
	}
	return http.StatusOK, nil
}

func RequestNewVolume(ctx context.Context, volume host.Volume) (*host.Volume, int, error) {
	if volume.Size == 0 {
		return nil, http.StatusBadRequest, errors.New("must specify volume size")
	}
	err := ValidVolumeOptions(&volume, evergreen.GetEnvironment().Settings())
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	mgr, err := GetEC2ManagerForVolume(ctx, &volume)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	vol, err := mgr.CreateVolume(ctx, &volume)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "creating volume")
	}
	return vol, http.StatusOK, nil
}

// CheckVolumeLimitExceeded checks if a user would exceed their volume size limit by adding newSize GB.
// If the limit would be exceeded, return an error.
func CheckVolumeLimitExceeded(ctx context.Context, user string, newSize int, maxSize int) error {
	totalSize, err := host.FindTotalVolumeSizeByUser(ctx, user)
	if err != nil {
		return errors.Wrapf(err, "counting total volume size for user '%s'", user)
	}
	if totalSize+newSize > maxSize {
		return errors.Errorf("cannot exceed user total volume size limit %d", maxSize)
	}
	return nil
}