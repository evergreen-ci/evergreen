package command

import (
	"regexp"
	"strings"
	"time"

	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
)

const (
	maxS3OpAttempts     = 5
	s3HTTPClientTimeout = 60 * time.Minute
	s3OpSleep           = 2 * time.Second
	s3OpRetryMaxSleep   = 20 * time.Second
)

var (
	// Regular expression for validating S3 bucket names
	bucketNameRegex = regexp.MustCompile(`^[A-Za-z0-9_\-.]+$`)
)

func validateS3BucketName(bucket string) error {
	// if it's an expandable string, we can't expand yet since we don't have
	// access to the task config expansions. So, we defer till during runtime
	// to do the validation
	if util.IsExpandable(bucket) {
		return nil
	}
	if len(bucket) < 3 {
		return errors.New("must be at least 3 characters")
	}
	if len(bucket) > 63 {
		return errors.New("must be no more than 63 characters")
	}
	if strings.HasPrefix(bucket, ".") || strings.HasPrefix(bucket, "-") {
		return errors.New("must not begin with a period or hyphen")
	}
	if strings.HasSuffix(bucket, ".") || strings.HasSuffix(bucket, "-") {
		return errors.New("must not end with a period or hyphen")
	}
	if strings.Contains(bucket, "..") {
		return errors.New("must not have two consecutive periods")
	}
	if !bucketNameRegex.MatchString(bucket) {
		return errors.New("must contain only combinations of uppercase/lowercase " +
			"letters, numbers, hyphens, underscores and periods")
	}
	return nil
}

func validS3Permissions(perm string) bool {
	perms := []string{
		string(s3Types.BucketCannedACLPrivate),
		string(s3Types.BucketCannedACLPublicRead),
		string(s3Types.BucketCannedACLPublicReadWrite),
		string(s3Types.BucketCannedACLAuthenticatedRead),
		string(s3Types.ObjectCannedACLBucketOwnerRead),
		string(s3Types.ObjectCannedACLBucketOwnerFullControl),
	}

	return utility.StringSliceContains(perms, perm)
}

func getS3OpBackoff() *backoff.Backoff {
	return &backoff.Backoff{
		Min:    s3OpSleep,
		Max:    s3OpRetryMaxSleep,
		Factor: 2,
		Jitter: true,
	}
}

func shouldRunForVariant(bvs []string, bv string) bool {
	return len(bvs) == 0 || utility.StringSliceContains(bvs, bv)
}

// getAssumedRoleARN checks if the provided session token
// is associated with an assumed role. If it is, it returns
// the role ARN.
func getAssumedRoleARN(conf *internal.TaskConfig, sessionToken string) string {
	if sessionToken == "" {
		return ""
	}

	return conf.AssumeRoleInformation[sessionToken].RoleARN
}

func getAssumedRoleInfo(conf *internal.TaskConfig, sessionToken string) *internal.AssumeRoleInformation {
	if sessionToken == "" {
		return nil
	}

	info, ok := conf.AssumeRoleInformation[sessionToken]
	if !ok {
		return nil
	}
	return &info
}

// getAssumedRoleExpiration checks if the provided session token
// is associated with an assumed role. If it is, it returns
// the expiration time of the role.
func getAssumedRoleExpiration(conf *internal.TaskConfig, sessionToken string) (time.Time, bool) {
	if sessionToken == "" {
		return time.Time{}, false
	}
	return conf.AssumeRoleInformation[sessionToken].Expiration, true
}
