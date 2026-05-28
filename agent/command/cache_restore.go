package command

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// cacheRestore is a command that downloads and extracts a previously saved
// cache artifact from S3, setting a cache-hit expansion the rest of the task
// can branch on.
type cacheRestore struct {
	cacheCommon `mapstructure:",squash" plugin:"expand"`
	base
}

func cacheRestoreFactory() Command   { return &cacheRestore{} }
func (c *cacheRestore) Name() string { return "cache.restore" }

func (c *cacheRestore) ParseParams(params map[string]any) error {
	if err := decodeCacheParams(params, c); err != nil {
		return err
	}
	return errors.Wrap(c.validate(), "validating params")
}

func (c *cacheRestore) validate() error {
	catcher := grip.NewSimpleCatcher()
	c.validateCommon(catcher)
	return catcher.Resolve()
}

func (c *cacheRestore) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	c.taskData = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := c.expandParams(c, conf); err != nil {
		return errors.Wrap(err, "expanding params")
	}
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating expanded params")
	}
	c.populateAssumedRole(conf)

	key, err := c.resolveCacheKey(conf)
	if err != nil {
		return errors.Wrap(err, "computing cache key")
	}

	remoteKey := c.remoteKey(key)

	logger.Task().Infof(ctx, "cache.restore: computed cache key '%s'.", key)
	logger.Task().Infof(ctx, "cache.restore: looking up cache at '%s/%s'.", c.Bucket, remoteKey)

	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)
	if err := c.createPailBucket(ctx, comm, httpClient, false); err != nil {
		return errors.Wrap(err, "connecting to S3")
	}
	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "checking bucket")
	}

	exists, err := c.bucket.Exists(ctx, remoteKey)
	if err != nil {
		return errors.Wrap(err, "checking for cache object")
	}
	if !exists {
		logger.Task().Infof(ctx, "cache.restore: cache miss for key '%s'.", key)
		setCacheHit(conf, c.CacheName, false)
		return nil
	}

	localPath, err := createTempCacheArchive(conf.WorkDir)
	if err != nil {
		return errors.Wrap(err, "creating local cache file")
	}
	defer func() {
		logger.Task().Error(ctx, errors.Wrapf(os.Remove(localPath), "removing local cache archive '%s'", localPath))
	}()

	if err := c.bucket.Download(ctx, remoteKey, localPath); err != nil {
		return errors.Wrapf(err, "downloading cache object '%s'", remoteKey)
	}

	// A 0-byte file is treated as a miss. This mirrors Evergreen's optional
	// s3.get behavior when the object is absent (DEVPROD-17632).
	info, err := os.Stat(localPath)
	if err != nil {
		return errors.Wrapf(err, "stating downloaded cache file '%s'", localPath)
	}
	if info.Size() == 0 {
		logger.Task().Infof(ctx, "cache.restore: cache miss (0-byte object) for key '%s'.", key)
		setCacheHit(conf, c.CacheName, false)
		return nil
	}

	if err := c.extract(ctx, localPath, conf.WorkDir); err != nil {
		return errors.Wrap(err, "extracting cache archive")
	}

	logger.Task().Infof(ctx, "cache.restore: cache hit for key '%s', extracted into '%s'.", key, conf.WorkDir)
	setCacheHit(conf, c.CacheName, true)
	return nil
}

func (c *cacheRestore) extract(ctx context.Context, archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrapf(err, "opening cache archive '%s'", archivePath)
	}
	defer f.Close()

	return errors.Wrap(extractTarball(ctx, f, dest, []string{}), "extracting tarball")
}
