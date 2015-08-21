package s3Plugin

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/s3"
	"regexp"
	"strings"
)

func init() {
	plugin.Publish(&S3Plugin{})
}

// S3Plugin handles uploading and downloading from Amazon's S3 service
type S3Plugin struct {
}

const (
	S3GetCmd     = "get"
	S3PutCmd     = "put"
	S3PluginName = "s3"
)

var (
	// Regular expression for validating S3 bucket names
	BucketNameRegex = regexp.MustCompile("^[A-Za-z0-9_\\-.]+$")
)

// Name returns the name of the plugin. Fulfills Plugin interface.
func (self *S3Plugin) Name() string {
	return S3PluginName
}

// NewCommand returns commands of the given name.
// Fulfills Plugin interface.
func (self *S3Plugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == S3PutCmd {
		return &S3PutCommand{}, nil
	}
	if cmdName == S3GetCmd {
		return &S3GetCommand{}, nil
	}
	return nil, fmt.Errorf("No such command: %v", cmdName)
}

func validateS3BucketName(bucket string) error {
	// if it's an expandable string, we can't expand yet since we don't have
	// access to the task config expansions. So, we defer till during runtime
	// to do the validation
	if plugin.IsExpandable(bucket) {
		return nil
	}
	if len(bucket) < 3 {
		return fmt.Errorf("must be at least 3 characters")
	}
	if len(bucket) > 63 {
		return fmt.Errorf("must be no more than 63 characters")
	}
	if strings.HasPrefix(bucket, ".") || strings.HasPrefix(bucket, "-") {
		return fmt.Errorf("must not begin with a period or hyphen")
	}
	if strings.HasSuffix(bucket, ".") || strings.HasSuffix(bucket, "-") {
		return fmt.Errorf("must not end with a period or hyphen")
	}
	if strings.Contains(bucket, "..") {
		return fmt.Errorf("must not have two consecutive periods")
	}
	if !BucketNameRegex.MatchString(bucket) {
		return fmt.Errorf("must contain only combinations of uppercase/lowercase " +
			"letters, numbers, hyphens, underscores and periods")
	}
	return nil
}

func validS3Permissions(perm string) bool {
	return util.SliceContains(
		[]s3.ACL{
			s3.Private,
			s3.PublicRead,
			s3.PublicReadWrite,
			s3.AuthenticatedRead,
			s3.BucketOwnerRead,
			s3.BucketOwnerFull,
		},
		s3.ACL(perm),
	)
}
