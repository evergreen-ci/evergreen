package thirdparty

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// githubFileCachePrefix is the key prefix; keys end in a content-addressed
	// blob SHA, so an object never changes under its key.
	githubFileCachePrefix = "github_file_cache/v1"

	githubFileCacheOutcomeAttribute = "evergreen.github.file_cache.outcome"
	githubFileCacheReasonAttribute  = "evergreen.github.file_cache.reason"
	githubFileCacheBytesAttribute   = "evergreen.github.file_cache.file_bytes"
	githubFileCacheGetMsAttribute   = "evergreen.github.file_cache.s3_get_ms"

	githubFileCacheOutcomeHit             = "hit"
	githubFileCacheOutcomeMissProduced    = "miss_produced"
	githubFileCacheOutcomeSHAUnresolvable = "sha_unresolvable"
	githubFileCacheOutcomeError           = "error"

	githubFileCacheReasonS3Get        = "s3_get"
	githubFileCacheReasonVerification = "verification_mismatch"
	githubFileCacheReasonS3Put        = "s3_put"
)

var (
	// githubFileCacheComponentRegexp matches characters safe to interpolate
	// into an S3 object key, mirroring validateSourceCacheRepoComponents.
	githubFileCacheComponentRegexp = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

	// githubFileCacheBucketFn is a variable so tests can inject a local bucket.
	githubFileCacheBucketFn = getGitHubFileCacheBucket
)

// githubFileCache holds the memoized cache bucket. It is rebuilt when the
// configured bucket name changes, which makes the bucket name the feature's
// runtime kill switch.
var githubFileCache struct {
	sync.Mutex
	bucketName string
	bucket     pail.Bucket
}

// getGitHubFileCacheBucket returns the configured cache bucket, or nil if the
// feature is disabled or unusable, so the cache can only save time, never fail
// a config read.
func getGitHubFileCacheBucket(ctx context.Context) pail.Bucket {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "getting admin settings for the GitHub file cache",
		}))
		return nil
	}
	bucketConfig := settings.Buckets.GitHubFileCacheBucket
	if bucketConfig.Name == "" {
		return nil
	}

	githubFileCache.Lock()
	defer githubFileCache.Unlock()
	if githubFileCache.bucket != nil && githubFileCache.bucketName == bucketConfig.Name {
		return githubFileCache.bucket
	}

	bucket, err := pail.NewS3MultiPartBucket(ctx, pail.S3Options{
		Name:          bucketConfig.Name,
		Region:        evergreen.DefaultEC2Region,
		AssumeRoleARN: bucketConfig.RoleARN,
	})
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "building the GitHub file cache bucket",
			"bucket":  bucketConfig.Name,
		}))
		return nil
	}
	githubFileCache.bucketName = bucketConfig.Name
	githubFileCache.bucket = bucket
	return bucket
}

// getGitHubFileWithCache returns the contents of file at HEAD of the git
// directory dir, reading through the blob-SHA-keyed cache when a bucket is
// configured. A hit skips GitRestoreFile's on-disk validation, which is fine
// because the verified bytes go only to the config parser, never to disk.
func getGitHubFileWithCache(ctx context.Context, span trace.Span, owner, repo, ref, dir, file string) ([]byte, error) {
	blobSHA, err := gitRevParseBlobSHA(ctx, dir, file)
	if err != nil {
		// The file most likely does not exist at HEAD; GitRestoreFile owns the
		// canonical file-not-found error.
		span.SetAttributes(attribute.String(githubFileCacheOutcomeAttribute, githubFileCacheOutcomeSHAUnresolvable))
		return GitRestoreFile(ctx, owner, repo, ref, dir, file)
	}

	bucket := githubFileCacheBucketFn(ctx)
	if bucket == nil {
		return GitRestoreFile(ctx, owner, repo, ref, dir, file)
	}

	if !validGitHubFileCacheComponents(owner, repo) {
		return GitRestoreFile(ctx, owner, repo, ref, dir, file)
	}

	key := githubFileCacheKey(owner, repo, blobSHA)
	content, hit := getGitHubFileCacheObject(ctx, span, bucket, key, blobSHA)
	if hit {
		return content, nil
	}

	fileContent, err := GitRestoreFile(ctx, owner, repo, ref, dir, file)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String(githubFileCacheOutcomeAttribute, githubFileCacheOutcomeMissProduced))
	putGitHubFileCacheObject(ctx, span, bucket, key, fileContent)
	return fileContent, nil
}

// gitRevParseBlobSHA resolves the blob SHA of file at HEAD; the <rev>:<path>
// syntax treats the path as a path, so no pathspec magic applies.
func gitRevParseBlobSHA(ctx context.Context, dir, file string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, GitOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", fmt.Sprintf("HEAD:%s", file))
	cmd.Dir = dir
	cmd.WaitDelay = gitOperationWaitDelay
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "resolving git blob SHA of file '%s' at HEAD", file)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// githubFileCacheKey keys by blob SHA alone, so the path is deliberately not
// part of the key.
func githubFileCacheKey(owner, repo, blobSHA string) string {
	return fmt.Sprintf("%s/%s/%s/%s", githubFileCachePrefix, owner, repo, blobSHA)
}

// validGitHubFileCacheComponents reports whether the owner and repo are safe to
// interpolate into a cache key.
func validGitHubFileCacheComponents(owner, repo string) bool {
	for _, component := range []string{owner, repo} {
		if component == "." || component == ".." || !githubFileCacheComponentRegexp.MatchString(component) {
			return false
		}
	}
	return true
}

// getGitHubFileCacheObject returns the object's contents if the key holds
// content that verifies against the key's blob SHA; any failure is a miss.
func getGitHubFileCacheObject(ctx context.Context, span trace.Span, bucket pail.Bucket, key, blobSHA string) ([]byte, bool) {
	start := time.Now()
	r, err := bucket.Get(ctx, key)
	var content []byte
	if err == nil {
		defer r.Close()
		content, err = io.ReadAll(r)
	}
	if pail.IsKeyNotFoundError(err) {
		return nil, false
	}
	if err != nil {
		setCacheError(span, githubFileCacheReasonS3Get)
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "reading file from the GitHub file cache",
			"key":     key,
		}))
		return nil, false
	}
	span.SetAttributes(attribute.Int(githubFileCacheGetMsAttribute, int(time.Since(start).Milliseconds())))

	if !verifyGitBlobContent(content, blobSHA) {
		setCacheError(span, githubFileCacheReasonVerification)
		grip.Warning(ctx, message.Fields{
			"message": "cached GitHub file content does not match its blob SHA, treating as a cache miss",
			"key":     key,
		})
		return nil, false
	}

	span.SetAttributes(
		attribute.String(githubFileCacheOutcomeAttribute, githubFileCacheOutcomeHit),
		attribute.Int(githubFileCacheBytesAttribute, len(content)),
	)
	return content, true
}

// putGitHubFileCacheObject stores the file contents in the cache. A PUT error
// is only logged; it must never fail the caller's request.
func putGitHubFileCacheObject(ctx context.Context, span trace.Span, bucket pail.Bucket, key string, content []byte) {
	if err := bucket.Put(ctx, key, bytes.NewReader(content)); err != nil {
		setCacheError(span, githubFileCacheReasonS3Put)
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "putting file into the GitHub file cache",
			"key":     key,
		}))
		return
	}
	span.SetAttributes(attribute.Int(githubFileCacheBytesAttribute, len(content)))
}

// setCacheError marks the span's cache outcome as an error with the given
// reason.
func setCacheError(span trace.Span, reason string) {
	span.SetAttributes(
		attribute.String(githubFileCacheOutcomeAttribute, githubFileCacheOutcomeError),
		attribute.String(githubFileCacheReasonAttribute, reason),
	)
}

// verifyGitBlobContent reports whether content hashes to the given git blob
// SHA, i.e. the SHA-1 of "blob <size>\0" followed by the content.
func verifyGitBlobContent(content []byte, blobSHA string) bool {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil)) == blobSHA
}
