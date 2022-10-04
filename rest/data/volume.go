package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

func MigrateVolume(ctx context.Context, volumeID string, options *restModel.HostRequestOptions, user *user.DBUser,
	env evergreen.Environment) (bool, error) {

	// Get key value if PublicKey is a name
	keyVal, err := user.GetPublicKey(options.KeyName)
	if err != nil {
		// if the keyname is populated but isn't a valid name, it may be the key value itself
		if options.KeyName == "" {
			return false, err
		}
		keyVal = options.KeyName
	}
	if keyVal == "" {
		return false, errors.Errorf("public key '%s' cannot have an empty value", options.KeyName)
	}
	spawnOptions := cloud.SpawnOptions{
		DistroId:              options.DistroID,
		Userdata:              options.UserData,
		UserName:              user.Username(),
		PublicKey:             keyVal,
		InstanceTags:          options.InstanceTags,
		InstanceType:          options.InstanceType,
		NoExpiration:          options.NoExpiration,
		IsVirtualWorkstation:  options.IsVirtualWorkstation,
		IsCluster:             options.IsCluster,
		HomeVolumeSize:        options.HomeVolumeSize,
		HomeVolumeID:          options.HomeVolumeID,
		Region:                options.Region,
		Expiration:            options.Expiration,
		UseProjectSetupScript: options.UseProjectSetupScript,
		ProvisionOptions: &host.ProvisionOptions{
			TaskId:      options.TaskID,
			TaskSync:    options.TaskSync,
			SetupScript: options.SetupScript,
			OwnerId:     user.Id,
		},
	}

	ts := utility.RoundPartOfMinute(0).Format(units.TSFormat)
	if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), units.NewVolumeMigrationJob(env, volumeID, spawnOptions, ts)); err != nil {
		return false, errors.Wrap(err, "enqueuing volume migration job")

	}
	return true, nil
}
