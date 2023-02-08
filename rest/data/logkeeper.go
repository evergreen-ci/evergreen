package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/plank"
)

func GetLogkeeperBuildMetadata(ctx context.Context, buildID string) (plank.Build, error) {
	opts := plank.NewLogkeeperClientOptions{
		BaseURL: evergreen.GetEnvironment().Settings().LoggerConfig.LogkeeperURL,
	}
	client := plank.NewLogkeeperClient(opts)
	return client.GetBuildMetadata(ctx, buildID)
}
