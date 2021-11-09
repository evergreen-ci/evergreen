package debgen_test

import (
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/debgen"
	"log"
	"os"
	"path/filepath"
)

func Example_genBinaryPackage() {

	ctrl := deb.NewControlDefault("testpkg", "me", "<a@me.org>", "Dummy package for doing nothing", "testpkg is a dummy package", true)
	for _, pkg := range ctrl.BinaryParas() {
		pkg.Set(deb.VersionFName, "0.0.2")
	}
	build := debgen.NewBuildParams()
	build.Init()
	build.IsRmtemp = false
	artifacts, err := deb.NewWriters(ctrl)
	if err != nil {
		log.Fatalf("Error building binary: %v", err)
	}
	artifacts[deb.ArchAmd64].MappedFiles = map[string]string{"/usr/bin/a": "_out/a.amd64"}
	artifacts[deb.ArchI386].MappedFiles = map[string]string{"/usr/bin/a": "_out/a.i386"}
	artifacts[deb.ArchArmhf].MappedFiles = map[string]string{"/usr/bin/a": "_out/a.armhf"}

	prep() //prepare files for packaging using some other means.

	for arch, artifact := range artifacts {
		log.Printf("generating artifact '%s'/%v", arch, artifact)
		dgen := debgen.NewDebGenerator(artifact, build)
		err = dgen.GenerateAllDefault()
		if err != nil {
			log.Fatalf("Error building for '%s': %v", arch, err)
		}
	}

	// Output:
	//
}

func prep() error {
	exesMap := map[string][]string{
		"amd64": []string{"_out/a.amd64"},
		"i386":  []string{"_out/a.i386"},
		"armhf": []string{"_out/a.armhf"}}
	err := createExes(exesMap)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return err
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
