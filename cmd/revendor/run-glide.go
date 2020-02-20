package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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

	updateGlide(pkg, revision)

	installer := repo.NewInstaller()
	action.EnsureGoVendor()
	action.Install(installer, false, false)
}

func updateGlide(pkg, revision string) {
	wd, err := os.Getwd()
	grip.EmergencyFatal(errors.Wrap(err, "error getting working dir"))

	vendorPath := filepath.Join(wd, "vendor", pkg)
	stat, err := os.Stat(vendorPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(vendorPath, 0777)
			grip.EmergencyFatal(errors.Wrapf(err, "unable to make directory %s", vendorPath))
		} else {
			grip.EmergencyFatal(errors.Wrapf(err, "vendor directory %s not found", vendorPath))
		}
	} else if !stat.IsDir() {
		grip.EmergencyFatal(fmt.Sprintf("'%s' is not a directory", vendorPath))
	}

	glidePath := filepath.Join(wd, "glide.lock")
	_, err = os.Stat(glidePath)
	grip.EmergencyFatal(errors.Wrapf(err, "glide file %s not found", glidePath))

	glideFile, err := os.Open(glidePath)
	grip.EmergencyFatal(errors.Wrap(err, "error opening glide file"))
	glide, err := ioutil.ReadAll(glideFile)
	grip.EmergencyFatal(errors.Wrap(err, "error reading glide file"))

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
		lines = append(lines, fmt.Sprintf(`  - name: %s`, pkg))
		lines = append(lines, fmt.Sprintf(`    version: %s`, revision))
	}

	grip.EmergencyFatal(errors.Wrap(ioutil.WriteFile(glidePath, []byte(strings.Join(lines, "\n")), 0777), "error writing glide file"))

	grip.EmergencyFatal(errors.Wrap(os.RemoveAll(vendorPath), "error removing vendor dir"))
}
