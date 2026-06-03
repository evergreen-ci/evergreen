package command

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// cacheSave is a command that bundles paths into a tarball and uploads it to S3
// so a later task can restore them via cache.restore. It no-ops when the
// corresponding cache.restore already reported a hit.
type cacheSave struct {
	cacheCommon `mapstructure:",squash" plugin:"expand"`

	// Paths are file or directory paths, relative to the working directory, to
	// bundle into the tarball.
	Paths []string `mapstructure:"paths" plugin:"expand"`

	base
}

func cacheSaveFactory() Command   { return &cacheSave{} }
func (c *cacheSave) Name() string { return evergreen.CacheSaveCommandName }

func (c *cacheSave) ParseParams(params map[string]any) error {
	if err := decodeCacheParams(params, c); err != nil {
		return err
	}
	return errors.Wrap(c.validate(), "validating params")
}

func (c *cacheSave) validate() error {
	catcher := grip.NewSimpleCatcher()
	c.validateCommon(catcher)
	catcher.NewWhen(len(c.Paths) == 0, "at least one paths value is required")
	return catcher.Resolve()
}

func (c *cacheSave) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	c.taskData = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := c.expandParams(c, conf); err != nil {
		return errors.Wrap(err, "expanding params")
	}
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating expanded params")
	}

	// The cache key is recomputed rather than read from an expansion that
	// cache.restore would have set, so the two commands aren't coupled through
	// hidden state. The hit expansion is only used to decide whether to skip.
	if cacheWasHit(conf, c.CacheName) {
		logger.Task().Infof(ctx, "cache.save: skipping save because cache '%s' was already a hit on restore.", c.CacheName)
		return nil
	}

	c.populateAssumedRole(conf)

	key, err := c.resolveCacheKey(conf)
	if err != nil {
		return errors.Wrap(err, "computing cache key")
	}

	remoteKey := c.remoteKey(key)

	localPath, err := createTempCacheArchive(conf.WorkDir)
	if err != nil {
		return errors.Wrap(err, "creating local cache file")
	}
	defer func() {
		logger.Task().Error(ctx, errors.Wrapf(os.Remove(localPath), "removing local cache archive '%s'", localPath))
	}()

	logger.Task().Infof(ctx, "cache.save: computed cache key '%s'.", key)
	logger.Task().Infof(ctx, "cache.save: bundling paths %s into '%s'.", c.Paths, localPath)

	if err := makeCacheArchive(ctx, conf.WorkDir, c.Paths, localPath, logger.Task()); err != nil {
		return errors.Wrap(err, "creating cache archive")
	}

	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)
	if err := c.createPailBucket(ctx, comm, httpClient, true); err != nil {
		return errors.Wrap(err, "connecting to S3")
	}
	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "checking bucket")
	}

	alreadyExists := false
	uploadDesc := fmt.Sprintf("upload cache object '%s'", remoteKey)
	err = retryS3Op(ctx, logger.Task(), uploadDesc, func() (bool, error) {
		uploadErr := c.bucket.Upload(ctx, remoteKey, localPath)
		if uploadErr == nil {
			return false, nil
		}
		// Skip-existing semantics: S3 reports PreconditionFailed when the
		// object already exists and IfNotExists is set on the request. That is
		// not an error for caching, it just means another task saved first.
		var apiErr smithy.APIError
		if errors.As(uploadErr, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
			alreadyExists = true
			return false, nil
		}
		// Other client errors (4xx) won't succeed on retry, so fail fast rather
		// than burning the full retry budget.
		if isS3ClientError(uploadErr) {
			return false, uploadErr
		}
		return true, uploadErr
	})
	if err != nil {
		return errors.Wrapf(err, "uploading cache object '%s'", remoteKey)
	}
	if alreadyExists {
		logger.Task().Infof(ctx, "cache.save: not uploading because '%s/%s' already exists.", c.Bucket, remoteKey)
		return nil
	}

	logger.Task().Infof(ctx, "cache.save: uploaded cache to '%s/%s'.", c.Bucket, remoteKey)
	return nil
}
