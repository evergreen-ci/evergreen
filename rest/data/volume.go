package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

func MigrateVolume(ctx context.Context, volumeID string, options *restModel.HostRequestOptions, user *user.DBUser,
	env evergreen.Environment) (bool, error) {
	spawnOptions, err := makeSpawnOptions(options, user)
	if err != nil {
		return false, err
	}

	ts := utility.RoundPartOfMinute(0).Format(units.TSFormat)
	if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), units.NewVolumeMigrationJob(env, volumeID, *spawnOptions, ts)); err != nil {
		return false, errors.Wrap(err, "enqueuing volume migration job")

	}
	return true, nil
}
