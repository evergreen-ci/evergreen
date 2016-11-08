package deb_test

import (
	"github.com/debber/debber-v0.3/deb"
	"log"
	"testing"
)

func Example_buildDevPackage() {

	ctrl := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	buildFunc := func(dpkg *deb.Control) error {
		// Generate files here.
		return nil
	}
	spara := ctrl.GetParasByField(deb.SourceFName, "testpkg")
	bpara := ctrl.GetParasByField(deb.PackageFName, "testpkg-dev")
	nctrl := deb.Control{spara[0], bpara[0]}
	err := buildFunc(&nctrl)
	if err != nil {
		log.Fatalf("%v", err)
	}
}

func Test_buildDevPackage(t *testing.T) {

	ctrl := deb.NewControlDefault("testpkg", "me", "me@a", "Dummy package for doing nothing", "testpkg is package ", true)
	buildFunc := func(dpkg *deb.Control) error {
		// Generate files here.
		return nil
	}
	spara := ctrl.GetParasByField(deb.SourceFName, "testpkg")
	bpara := ctrl.GetParasByField(deb.PackageFName, "testpkg-dev")
	nctrl := deb.Control{spara[0], bpara[0]}
	err := buildFunc(&nctrl)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
