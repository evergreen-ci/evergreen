package deb_test

import (
	"github.com/debber/debber-v0.3/deb"
	"github.com/debber/debber-v0.3/targz"
	"io/ioutil"
	"log"
	"path/filepath"
	"testing"
)

func Example_buildSourceDeb() {
	pkg := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	spkg := deb.NewSourcePackage(pkg)
	err := buildOrigArchive(spkg) // it's up to you how to build this
	if err != nil {
		log.Fatalf("Error building source package: %v", err)
	}
	err = buildDebianArchive(spkg) // again - do it yourself
	if err != nil {
		log.Fatalf("Error building source package: %v", err)
	}
	err = buildDscFile(spkg) // yep, same again
	if err != nil {
		log.Fatalf("Error building source package: %v", err)
	}
}

func Test_buildSourceDeb(t *testing.T) {
	pkg := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	spkg := deb.NewSourcePackage(pkg)
	err := buildOrigArchive(spkg) // it's up to you how to build this
	if err != nil {
		t.Fatalf("Error building source package: %v", err)
	}
	err = buildDebianArchive(spkg) // again - do it yourself
	if err != nil {
		t.Fatalf("Error building source package: %v", err)
	}
	err = buildDscFile(spkg) // yep, same again
	if err != nil {
		t.Fatalf("Error building source package: %v", err)
	}
}

func buildOrigArchive(spkg *deb.SourcePackage) error {
	origFilePath := filepath.Join(deb.DistDirDefault, spkg.OrigFileName)
	tgzw, err := targz.NewWriterFromFile(origFilePath)
	if err != nil {
		return err
	}
	// Add Sources Here !!
	err = tgzw.Close()
	if err != nil {
		return err
	}
	return nil
}

func buildDebianArchive(spkg *deb.SourcePackage) error {
	tgzw, err := targz.NewWriterFromFile(filepath.Join(deb.DistDirDefault, spkg.DebianFileName))
	if err != nil {
		return err
	}
	// Add Control Files here !!
	err = tgzw.Close()
	if err != nil {
		return err
	}
	return nil
}

func buildDscFile(spkg *deb.SourcePackage) error {
	dscData := []byte{} //generate this somehow. DIY (or see 'debgen' package in this repository)!
	dscFilePath := filepath.Join(deb.DistDirDefault, spkg.DscFileName)
	err := ioutil.WriteFile(dscFilePath, dscData, 0644)
	if err != nil {
		return err
	}
	return nil
}
