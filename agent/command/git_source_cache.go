package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// sourceCachePrefix is the top-level S3 key prefix for source cache artifacts.
const sourceCachePrefix = "source_cache/v1"

// sourceCachePRNamespace isolates artifacts produced by PR or merge queue tasks
// from base-revision artifacts to enforce trust boundaries.
const sourceCachePRNamespace = "pr"

// sourceCacheRegion is the region the source cache bucket lives in.
const sourceCacheRegion = evergreen.DefaultEC2Region

var (
	sourceCacheOutcomeAttribute           = sourceCacheAttribute("outcome")
	sourceCacheReasonAttribute            = sourceCacheAttribute("reason")
	sourceCacheKeyAttribute               = sourceCacheAttribute("cache_key")
	sourceCacheOwnerAttribute             = sourceCacheAttribute("owner")
	sourceCacheRepoAttribute              = sourceCacheAttribute("repo")
	sourceCacheRevisionAttribute          = sourceCacheAttribute("revision")
	sourceCacheRestoredRevisionAttribute  = sourceCacheAttribute("restored_revision")
	sourceCacheCloneDepthAttribute        = sourceCacheAttribute("clone_depth")
	sourceCacheRecurseSubmodulesAttribute = sourceCacheAttribute("recurse_submodules")
	sourceCacheArtifactBytesAttribute     = sourceCacheAttribute("artifact_bytes")

	sourceCacheCloneDurationAttribute    = sourceCacheAttribute("clone_duration_ms")
	sourceCacheDownloadDurationAttribute = sourceCacheAttribute("download_duration_ms")
	sourceCacheExtractDurationAttribute  = sourceCacheAttribute("extract_duration_ms")
	sourceCacheArchiveDurationAttribute  = sourceCacheAttribute("archive_duration_ms")
	sourceCacheUploadDurationAttribute   = sourceCacheAttribute("upload_duration_ms")
)

func sourceCacheAttribute(name string) string {
	return fmt.Sprintf("%s.source_cache.%s", gitGetProjectAttribute, name)
}

// Source cache outcomes reported on the command's span.
const (
	sourceCacheHit          = "hit"
	sourceCacheMissProduced = "miss_produced"
	sourceCacheMissLostRace = "miss_lost_race"
	sourceCacheSkipped      = "skipped"
	sourceCacheFallback     = "fallback"
)

// sourceCache round-trips a task's cloned project directory through S3 on behalf
// of git.get_project. It keys on (owner, repo, revision, clone shape) so every
// version built on a commit shares one artifact.
type sourceCache struct {
	cfg               evergreen.BucketConfig
	taskData          client.TaskData
	workDir           string
	dir               string
	owner, repo       string
	branch            string
	baseRevision      string
	prRevision        string
	revision          string
	cloneDepth        int
	recurseSubmodules bool

	key              string
	remoteKey        string
	corruptRemoteKey string
}

// newSourceCache returns the source cache for this run, or a nil cache and the
// reason it is off, given the agent's GOOS. It never returns an error the
// command should fail on.
func newSourceCache(conf *internal.TaskConfig, c *gitFetchProject, opts cloneOpts, goos string) (*sourceCache, string) {
	if goos != "linux" {
		// Producers and consumers must share a platform because the key holds
		// no platform component and working-tree materialization differs.
		return nil, "the source cache is only enabled on Linux agents"
	}
	if conf.SourceCacheBucket.Name == "" {
		return nil, "no source cache bucket is configured for this project"
	}
	if c.Filter != "" || len(c.SparseCheckoutPaths) > 0 {
		// A restored partial clone keeps a promisor remote whose lazy fetches
		// would run later with an expired token.
		return nil, "the source cache is skipped for partial and sparse clones"
	}
	if conf.Task.Revision == "" {
		return nil, "the task has no revision to key the source cache on"
	}

	sc := &sourceCache{
		cfg:               conf.SourceCacheBucket,
		taskData:          conf.TaskData(),
		workDir:           conf.WorkDir,
		dir:               c.Directory,
		owner:             opts.owner,
		repo:              opts.repo,
		branch:            opts.branch,
		baseRevision:      conf.Task.Revision,
		cloneDepth:        opts.cloneDepth,
		recurseSubmodules: opts.recurseSubmodules,
	}
	if pr := prCheckoutCommit(conf); pr != conf.Task.Revision {
		sc.prRevision = pr
	}
	sc.revision = sc.restoreRevisions()[0]

	key, remoteKey, err := sc.cacheKeysForRevision(sc.revision)
	if err != nil {
		return nil, fmt.Sprintf("computing the source cache key: %s", err)
	}
	sc.key = key
	sc.remoteKey = remoteKey
	return sc, ""
}

// restoreRevisions returns candidate lookup revisions, most specific first.
func (sc *sourceCache) restoreRevisions() []string {
	if sc.prRevision != "" {
		return []string{sc.prRevision, sc.baseRevision}
	}
	return []string{sc.baseRevision}
}

func (sc *sourceCache) namespaceForRevision(revision string) string {
	if revision == sc.prRevision {
		return sourceCachePRNamespace
	}
	return ""
}

// keyFor returns the content key and S3 object key for a revision.
func (sc *sourceCache) cacheKeysForRevision(revision string) (string, string, error) {
	namespace := sc.namespaceForRevision(revision)
	// A PR artifact is already pinned to the PR head, so the branch would only fragment its keys.
	branch := sc.branch
	if namespace == sourceCachePRNamespace {
		branch = ""
	}
	expansions := []string{namespace, sc.owner, sc.repo, branch, revision, strconv.Itoa(sc.cloneDepth), strconv.FormatBool(sc.recurseSubmodules)}
	key, err := computeCacheKey(nil, expansions, true)
	if err != nil {
		return "", "", err
	}
	return key, path.Join(sourceCachePrefix, sc.owner, sc.repo, namespace, revision, key+cacheArchiveSuffix), nil
}

func (sc *sourceCache) projectDir() string {
	return filepath.Join(sc.workDir, sc.dir)
}

func (sc *sourceCache) createBucket(ctx context.Context, comm client.Communicator, httpClient *http.Client, ifNotExists bool) (pail.Bucket, error) {
	opts := pail.S3Options{
		Region:      sourceCacheRegion,
		Name:        sc.cfg.Name,
		IfNotExists: ifNotExists,
	}
	if sc.cfg.RoleARN != "" {
		opts.Credentials = newCachedEvergreenCredentials(comm, sc.taskData, nil, sc.cfg.RoleARN, nil)
	}
	bucket, err := pail.NewS3MultiPartBucketWithHTTPClient(ctx, httpClient, opts)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to S3")
	}
	return bucket, errors.Wrap(bucket.Check(ctx), "checking bucket")
}

// restore downloads and extracts the cached source tree at remoteKey.
func (sc *sourceCache) restore(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, remoteKey string) (bool, error) {
	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)

	bucket, err := sc.createBucket(ctx, comm, httpClient, false)
	if err != nil {
		return false, err
	}

	localPath, err := createTempCacheArchive(sc.workDir)
	if err != nil {
		return false, errors.Wrap(err, "creating local cache file")
	}
	defer func() {
		logger.Task().Error(ctx, errors.Wrapf(os.Remove(localPath), "removing local cache archive '%s'", localPath))
	}()

	start := time.Now()
	miss := false
	downloadDesc := fmt.Sprintf("download cache object '%s'", remoteKey)
	err = retryS3Op(ctx, logger.Task(), downloadDesc, func() (bool, error) {
		downloadErr := bucket.Download(ctx, remoteKey, localPath)
		if downloadErr == nil {
			return false, nil
		}
		switch classifyCacheDownloadErr(downloadErr) {
		case cacheDownloadMaybeMiss:
			logger.Task().Warningf(ctx, "git source cache: got access-denied downloading '%s/%s', treating as a miss.", sc.cfg.Name, remoteKey)
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
		return false, errors.Wrapf(err, "downloading cache object '%s'", remoteKey)
	}
	if miss {
		return false, nil
	}
	setSourceCacheSpanDuration(ctx, sourceCacheDownloadDurationAttribute, time.Since(start))

	info, err := os.Stat(localPath)
	if err != nil {
		return false, errors.Wrapf(err, "stating downloaded source cache file '%s'", localPath)
	}
	if info.Size() == 0 {
		return false, nil
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int64(sourceCacheArtifactBytesAttribute, info.Size()))

	f, err := os.Open(localPath)
	if err != nil {
		return false, errors.Wrapf(err, "opening source cache archive '%s'", localPath)
	}
	defer f.Close()

	// The clone path starts by removing the project directory, so the restore
	// path does too rather than extracting over whatever is there.
	if err := os.RemoveAll(sc.projectDir()); err != nil {
		return false, errors.Wrapf(err, "removing project directory '%s'", sc.dir)
	}
	// The archive holds no entry for the project directory itself, so recreate
	// it with the mode the clone path leaves behind.
	if err := os.MkdirAll(sc.projectDir(), 0755); err != nil {
		return false, errors.Wrapf(err, "creating project directory '%s'", sc.dir)
	}

	start = time.Now()
	if err := sc.extractArchive(ctx, f, remoteKey); err != nil {
		return false, err
	}
	setSourceCacheSpanDuration(ctx, sourceCacheExtractDurationAttribute, time.Since(start))
	return true, nil
}

// extractArchive unpacks a downloaded artifact into the project directory.
func (sc *sourceCache) extractArchive(ctx context.Context, r io.Reader, remoteKey string) error {
	if err := extractTarball(ctx, r, sc.projectDir(), []string{}, true); err != nil {
		sc.corruptRemoteKey = remoteKey
		return errors.Wrap(err, "extracting source cache archive")
	}
	return nil
}

// healsCorruptArtifact reports whether save should overwrite a corrupt artifact.
func (sc *sourceCache) healsCorruptArtifact() bool {
	return sc.corruptRemoteKey != "" && sc.corruptRemoteKey == sc.remoteKey
}

// save archives and uploads the project directory.
func (sc *sourceCache) save(ctx context.Context, comm client.Communicator, logger client.LoggerProducer) (bool, error) {
	localPath, err := createTempCacheArchive(sc.workDir)
	if err != nil {
		return false, errors.Wrap(err, "creating local cache file")
	}
	defer func() {
		logger.Task().Error(ctx, errors.Wrapf(os.Remove(localPath), "removing local cache archive '%s'", localPath))
	}()

	start := time.Now()
	if err := makeCacheArchive(ctx, sc.projectDir(), []string{sc.projectDir()}, localPath, logger.Task(), true); err != nil {
		return false, errors.Wrap(err, "creating source cache archive")
	}
	setSourceCacheSpanDuration(ctx, sourceCacheArchiveDurationAttribute, time.Since(start))
	if info, err := os.Stat(localPath); err == nil {
		trace.SpanFromContext(ctx).SetAttributes(attribute.Int64(sourceCacheArtifactBytesAttribute, info.Size()))
	}

	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)

	heal := sc.healsCorruptArtifact()
	if heal {
		logger.Task().Warningf(ctx, "Overwriting the corrupt source cache artifact at '%s'.", sc.remoteKey)
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Bool(sourceCacheAttribute("healed_corrupt_artifact"), heal))

	bucket, err := sc.createBucket(ctx, comm, httpClient, !heal)
	if err != nil {
		return false, err
	}

	start = time.Now()
	alreadyExists := false
	uploadDesc := fmt.Sprintf("upload cache object '%s'", sc.remoteKey)
	err = retryS3Op(ctx, logger.Task(), uploadDesc, func() (bool, error) {
		uploadErr := bucket.Upload(ctx, sc.remoteKey, localPath)
		if uploadErr == nil {
			return false, nil
		}
		var apiErr smithy.APIError
		if errors.As(uploadErr, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
			alreadyExists = true
			return false, nil
		}
		if isS3ClientError(uploadErr) {
			return false, uploadErr
		}
		return true, uploadErr
	})
	if err != nil {
		return false, errors.Wrapf(err, "uploading cache object '%s'", sc.remoteKey)
	}
	setSourceCacheSpanDuration(ctx, sourceCacheUploadDurationAttribute, time.Since(start))
	return !alreadyExists, nil
}

func setSourceCacheSpanDuration(ctx context.Context, attr string, d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	trace.SpanFromContext(ctx).SetAttributes(attribute.Float64(attr, ms))
}

// setSpanOutcome records the outcome of the source cache path, with the reason
// for the outcomes that have one.
func (sc *sourceCache) setSpanOutcome(ctx context.Context, outcome, reason string) {
	attrs := []attribute.KeyValue{
		attribute.String(sourceCacheOutcomeAttribute, outcome),
		attribute.String(sourceCacheKeyAttribute, sc.key),
		attribute.String(sourceCacheOwnerAttribute, sc.owner),
		attribute.String(sourceCacheRepoAttribute, sc.repo),
		attribute.String(sourceCacheRevisionAttribute, sc.revision),
		attribute.Int(sourceCacheCloneDepthAttribute, sc.cloneDepth),
		attribute.Bool(sourceCacheRecurseSubmodulesAttribute, sc.recurseSubmodules),
	}
	if reason != "" {
		attrs = append(attrs, attribute.String(sourceCacheReasonAttribute, reason))
	}
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
}

// setSpanSkipped records a skip and its reason for a run that never built a
// source cache, so disabled runs stay queryable alongside the rest.
func setSourceCacheSpanSkipped(ctx context.Context, reason string) {
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String(sourceCacheOutcomeAttribute, sourceCacheSkipped),
		attribute.String(sourceCacheReasonAttribute, reason),
	)
}
