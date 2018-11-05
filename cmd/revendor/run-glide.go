package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Masterminds/glide/action"
	"github.com/Masterminds/glide/repo"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func main() {
	var (
		pkg      string
		revision string
	)
	flag.StringVar(&pkg, "package", "", "name of the vendored package to update")
	flag.StringVar(&revision, "revision", "", "revision to which to update")
	flag.Parse()
	if pkg == "" || revision == "" {
		grip.Info("no repo to revendor, run-glide will no-op")
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		grip.Error(errors.Wrap(err, "error getting working dir"))
		return
	}
	vendorPath := fmt.Sprintf("%s/vendor/%s", wd, pkg)
	_, err = os.Stat(vendorPath)
	if os.IsNotExist(err) {
		grip.Errorf("vendor directory %s not found", vendorPath)
		return
	}
	glidePath := fmt.Sprintf("%s/glide.lock", wd)
	_, err = os.Stat(glidePath)
	if os.IsNotExist(err) {
		grip.Errorf("glide file %s not found", glidePath)
		return
	}

	glideFile, err := os.Open(glidePath)
	if err != nil {
		grip.Error(errors.Wrap(err, "error opening glide file"))
		return
	}
	glide, err := ioutil.ReadAll(glideFile)
	if err != nil {
		grip.Error(errors.Wrap(err, "error reading glide file"))
		return
	}
	lines := strings.Split(string(glide), "\n")
	found := false
	for i, line := range lines {
		if strings.Contains(line, pkg) {
			lines[i+1] = fmt.Sprintf("    version: %s", revision)
			found = true
			break
		}
	}
	if !found {
		grip.Errorf("package %s not found in glide file", pkg)
		return
	}
	err = ioutil.WriteFile(glidePath, []byte(strings.Join(lines, "\n")), 0777)
	if err != nil {
		grip.Error(errors.Wrap(err, "error writing glide file"))
		return
	}

	err = os.RemoveAll(vendorPath)
	if err != nil {
		grip.Error(errors.Wrap(err, "error removing vendor dir"))
		return
	}

	installer := repo.NewInstaller()
	action.EnsureGoVendor()
	action.Install(installer, false, false)
}
