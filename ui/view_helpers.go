package ui

import (
	"crypto/md5"
	"fmt"
	"html/template"
	"io"
	"os/exec"

	"github.com/evergreen-ci/evergreen/auth"
)

const (
	WebRootPath  = "ui"
	Templates    = "templates"
	Static       = "static"
	DefaultSkip  = 0
	DefaultLimit = 10
)

type OtherPageData map[string]interface{}

type pageData struct {
	ActiveItem         string
	Data               interface{}
	User               auth.User
	Project            string
	ProjectDisplayName string
	Other              OtherPageData
}

type pageContent struct {
	CSSFiles    []string
	JSFiles     []string
	MenuHTML    template.HTML
	ContentHTML template.HTML
}

// DirectoryChecksum compute an MD5 of a directory's contents. If a file in the directory changes,
// the hash will be different.
func DirectoryChecksum(home string) (string, error) {
	// Compute md5 of ui statics directory
	staticsDirectoryDetailsCmd := exec.Command("ls", "-lR", home)
	staticsDirectoryDetails, err := staticsDirectoryDetailsCmd.Output()
	if err != nil {
		return "", err
	}

	hash := md5.New()
	io.WriteString(hash, string(staticsDirectoryDetails))
	// fmt.Sprintf("%x") is magic for getting an actual checksum out of crypto/md5
	staticsMD5 := fmt.Sprintf("%x", hash.Sum(nil))

	return staticsMD5, nil
}
