package command

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/jpillora/backoff"
)

const (
	maxS3OpAttempts   = 10
	s3OpSleep         = 2 * time.Second
	s3OpRetryMaxSleep = 20 * time.Second
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
		string(s3.BucketCannedACLPrivate),
		string(s3.BucketCannedACLPublicRead),
		string(s3.BucketCannedACLPublicReadWrite),
		string(s3.BucketCannedACLAuthenticatedRead),
		string(s3.ObjectCannedACLBucketOwnerRead),
		string(s3.ObjectCannedACLBucketOwnerFullControl),
	}

	return util.StringSliceContains(perms, perm)
}

func getS3OpBackoff() *backoff.Backoff {
	return &backoff.Backoff{
		Min:    s3OpSleep,
		Max:    s3OpRetryMaxSleep,
		Factor: 2,
		Jitter: true,
	}
}

func s3BaseURL(bucketName string) string {
	return fmt.Sprintf("https://%s.s3.amazonaws.com", bucketName)
}
