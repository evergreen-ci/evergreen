package s3

import (
	"crypto/md5"
	"io"
	"log"
	"os"
	"strings"
)

func contentMd5(s string) (ret string, e error) {
	digest := md5.New()
	_, e = io.Copy(digest, strings.NewReader(s))
	if e != nil {
		return "", e
	}
	sum := digest.Sum(nil)
	return b64.EncodeToString(sum), nil
}

var logger = log.New(os.Stdout, "", 0)
