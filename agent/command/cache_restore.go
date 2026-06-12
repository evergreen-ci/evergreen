package command

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/pail"
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
func (c *cacheRestore) Name() string { return evergreen.CacheRestoreCommandName }

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

	localPath, err := createTempCacheArchive(conf.WorkDir)
	if err != nil {
		return errors.Wrap(err, "creating local cache file")
	}
	defer func() {
		logger.Task().Error(ctx, errors.Wrapf(os.Remove(localPath), "removing local cache archive '%s'", localPath))
	}()

	// Download directly rather than checking Exists first: a missing object
	// surfaces as a terminal cache miss, so we avoid an extra S3 round-trip (and
	// a HEAD request would face the same 403/404 ambiguity handled below).
	miss := false
	downloadDesc := fmt.Sprintf("download cache object '%s'", remoteKey)
	err = retryS3Op(ctx, logger.Task(), downloadDesc, func() (bool, error) {
		downloadErr := c.bucket.Download(ctx, remoteKey, localPath)
		if downloadErr == nil {
			return false, nil
		}
		switch classifyCacheDownloadErr(downloadErr) {
		case cacheDownloadMaybeMiss:
			logger.Task().Warningf(ctx, "cache.restore: got access-denied downloading '%s/%s', treating as a cache miss; if a cache was expected here, verify the credentials grant s3:GetObject on this path.", c.Bucket, remoteKey)
			miss = true
			return false, nil
		case cacheDownloadMiss:
			miss = true
			return false, nil
		case cacheDownloadFatal:
			return false, downloadErr
		default:
			return true, downloadErr
		}
	})
	if err != nil {
		return errors.Wrapf(err, "downloading cache object '%s'", remoteKey)
	}
	if miss {
		logger.Task().Infof(ctx, "cache.restore: cache miss for key '%s'.", key)
		setCacheHit(conf, c.CacheName, false)
		return nil
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

// cacheDownloadOutcome classifies a cache object download error so the retry
// loop can decide whether to retry, treat it as a miss, or fail.
type cacheDownloadOutcome int

const (
	// cacheDownloadRetry is a transient error worth retrying.
	cacheDownloadRetry cacheDownloadOutcome = iota
	// cacheDownloadFatal is a non-retryable error that should fail the command.
	cacheDownloadFatal
	// cacheDownloadMiss means the object is absent, a normal cache miss.
	cacheDownloadMiss
	// cacheDownloadMaybeMiss is an access-denied response. It's treated as a
	// miss but warned about, since it may instead be a permission
	// misconfiguration (see classifyCacheDownloadErr).
	cacheDownloadMaybeMiss
)

// classifyCacheDownloadErr decides how cache.restore should react to a non-nil
// download error. A 404 NoSuchKey is a clean miss, but S3 only returns it when
// the credentials hold bucket-wide s3:ListBucket; caches scoped to a bucket
// sub-path typically lack that, so S3 masks a missing object as a 403
// AccessDenied, which is treated as a (warned) miss. Other client (4xx) errors
// won't succeed on retry and are fatal; everything else is retried.
func classifyCacheDownloadErr(err error) cacheDownloadOutcome {
	if pail.IsKeyNotFoundError(err) {
		return cacheDownloadMiss
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "AccessDenied" {
		return cacheDownloadMaybeMiss
	}
	if isS3ClientError(err) {
		return cacheDownloadFatal
	}
	return cacheDownloadRetry
}

func (c *cacheRestore) extract(ctx context.Context, archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrapf(err, "opening cache archive '%s'", archivePath)
	}
	defer f.Close()

	return errors.Wrap(extractTarball(ctx, f, dest, []string{}, c.PreserveSymlinks), "extracting tarball")
}
