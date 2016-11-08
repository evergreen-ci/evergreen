package deb_test

import (
	"archive/tar"
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/targz"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Example_buildBinaryDeb() {

	pkg := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	exesMap := map[string][]string{
		"amd64": []string{filepath.Join(deb.TempDirDefault, "/a.amd64")},
		"i386":  []string{filepath.Join(deb.TempDirDefault, "/a.i386")},
		"armhf": []string{filepath.Join(deb.TempDirDefault, "/a.armhf")}}
	err := createExes(exesMap)
	if err != nil {
		log.Fatalf("%v", err)
	}
	artifacts, err := deb.NewWriters(pkg)
	if err != nil {
		log.Fatalf("Error building binary: %v", err)
	}
	artifacts[deb.ArchAmd64].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.amd64")}
	artifacts[deb.ArchI386].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.i386")}
	artifacts[deb.ArchArmhf].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.armhf")}
	buildDeb := func(art *deb.Writer) error {
		//generate artifact here ...
		return nil
	}
	for arch, artifact := range artifacts {
		//build binary deb here ...
		err = buildDeb(artifact)
		if err != nil {
			log.Fatalf("Error building for '%s': %v", arch, err)
		}
	}
}

func Test_buildBinaryDeb(t *testing.T) {

	pkg := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	exesMap := map[string][]string{
		"amd64": []string{filepath.Join(deb.TempDirDefault, "a.amd64")},
		"i386":  []string{filepath.Join(deb.TempDirDefault, "a.i386")},
		"armhf": []string{filepath.Join(deb.TempDirDefault, "a.armhf")}}
	err := createExes(exesMap)
	if err != nil {
		t.Fatalf("%v", err)
	}
	artifacts, err := deb.NewWriters(pkg)
	if err != nil {
		t.Fatalf("Error building binary: %v", err)
	}
	artifacts[deb.ArchAmd64].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.amd64")}
	artifacts[deb.ArchI386].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.i386")}
	artifacts[deb.ArchArmhf].MappedFiles = map[string]string{"/usr/bin/a": filepath.Join(deb.TempDirDefault, "/a.armhf")}
	buildDeb := func(art *deb.Writer) error {
		archiveFilename := filepath.Join(deb.TempDirDefault, art.ControlArchive)
		controlTgzw, err := targz.NewWriterFromFile(archiveFilename)
		if err != nil {
			return err
		}
		controlData := []byte("Package: testpkg\n")
		//TODO add more files here ...
		header := &tar.Header{Name: "control", Size: int64(len(controlData)), Mode: int64(644), ModTime: time.Now()}
		err = controlTgzw.WriteHeader(header)
		if err != nil {
			return err
		}
		_, err = controlTgzw.Write(controlData)
		if err != nil {
			return err
		}
		err = controlTgzw.Close()
		if err != nil {
			return err
		}

		archiveFilename = filepath.Join(deb.TempDirDefault, art.DataArchive)
		dataTgzw, err := targz.NewWriterFromFile(archiveFilename)
		if err != nil {
			return err
		}
		//TODO add files here ...
		err = dataTgzw.Close()
		if err != nil {
			return err
		}
		err = os.MkdirAll(deb.DistDirDefault, 0777)
		if err != nil {
			return err
		}
		//generate artifact here ...
		err = art.Build(deb.TempDirDefault, deb.DistDirDefault)
		if err != nil {
			return err
		}
		return nil
	}
	for arch, artifact := range artifacts {
		//build binary deb here ...
		err = buildDeb(artifact)
		if err != nil {
			t.Fatalf("Error building for '%s': %v", arch, err)
		}
	}
}

func createExes(exesMap map[string][]string) error {
	for _, exes := range exesMap {
		for _, exe := range exes {
			err := os.MkdirAll(filepath.Dir(exe), 0777)
			if err != nil {
				return err
			}
			fi, err := os.Create(exe)
			if err != nil {
				return err
			}
			_, err = fi.Write([]byte("echo 1"))
			if err != nil {
				return err
			}
			err = fi.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
