package service

import (
	"crypto/md5"
	"fmt"
	"io"
	"os/exec"

	"github.com/pkg/errors"
)

const (
	WebRootPath  = "service"
	Templates    = "templates"
	Static       = "static"
	DefaultSkip  = 0
	DefaultLimit = 10
)

type OtherPageData map[string]interface{}

// DirectoryChecksum compute an MD5 of a directory's contents. If a file in the directory changes,
// the hash will be different.
func DirectoryChecksum(home string) (string, error) {
	// Compute md5 of ui statics directory
	staticsDirectoryDetailsCmd := exec.Command("ls", "-lR", home)
	staticsDirectoryDetails, err := staticsDirectoryDetailsCmd.Output()
	if err != nil {
		return "", errors.WithStack(err)
	}

	hash := md5.New()
	if _, err = io.WriteString(hash, string(staticsDirectoryDetails)); err != nil {
		return "", errors.WithStack(err)

	}
	// fmt.Sprintf("%x") is magic for getting an actual checksum out of crypto/md5
	staticsMD5 := fmt.Sprintf("%x", hash.Sum(nil))

	return staticsMD5, nil
}
