package aws

import (
	"fmt"
	"os"
)

const (
	CONTENT_MD5            = "Content-Md5"
	CONTENT_TYPE           = "Content-Type"
	CONTENT_EXPIRES        = "Expires"
	CONTENT_ACL            = "x-amz-acl"
	ENV_AWS_ACCESS_KEY     = "AWS_ACCESS_KEY_ID"
	ENV_AWS_SECRET_KEY     = "AWS_SECRET_ACCESS_KEY"
	ENV_AWS_DEFAULT_REGION = "AWS_DEFAULT_REGION"
)

func abortWith(message string) {
	fmt.Println("ERROR: " + message)
	os.Exit(1)
}

// copied from https://launchpad.net/goamz
var unreserved = make([]bool, 128)
var hex = "0123456789ABCDEF"

func init() {
	// RFC3986
	u := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz01234567890-_.~"
	for _, c := range u {
		unreserved[c] = true
	}
}

func Encode(s string) string {
	encode := false
	for i := 0; i != len(s); i++ {
		c := s[i]
		if c > 127 || !unreserved[c] {
			encode = true
			break
		}
	}
	if !encode {
		return s
	}
	e := make([]byte, len(s)*3)
	ei := 0
	for i := 0; i != len(s); i++ {
		c := s[i]
		if c > 127 || !unreserved[c] {
			e[ei] = '%'
			e[ei+1] = hex[c>>4]
			e[ei+2] = hex[c&0xF]
			ei += 3
		} else {
			e[ei] = c
			ei += 1
		}
	}
	return string(e[:ei])
}
