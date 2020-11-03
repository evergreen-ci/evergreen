package util

import (
	"fmt"
	"strings"
)

// S3PathURL returns the path-style S3 URL for the given bucket containing the
// object key.
func S3PathURL(bucket, key string) string {
	return strings.Join([]string{"https://s3.amazonaws.com", bucket, key}, "/")
}

// S3VirtualHostedURL returns the virtual hosted-style S3 URL for the given
// bucket containing the object key.
func S3VirtualHostedURL(bucket, key string) string {
	return strings.Join([]string{fmt.Sprintf("https://%s.s3.amazonaws.com", bucket), key}, "/")
}

// S3DefaultURL returns the S3 URL for the given bucket containing the object
// key. The style of the S3 URL is determined based on the bucket name.
func S3DefaultURL(bucket, key string) string {
	if strings.Contains(bucket, ".") {
		return S3PathURL(bucket, key)
	}
	return S3VirtualHostedURL(bucket, key)
}
