package deb_test

import (
	"github.com/debber/debber-v0.3/deb"
	"os"
	"path/filepath"
	"testing"
)

var testControlFiles = []string{
	filepath.Join("testdata", "butaca.control"),
	filepath.Join("testdata", "gitso.control"),
	filepath.Join("testdata", "kompas-plugins.control"),
	filepath.Join("testdata", "xkcdMeegoReader.control"),
}

func TestParseControlFile(t *testing.T) {
	tcf2 := []string{}
	copy(tcf2, testControlFiles)
	tcf2 = append(tcf2, filepath.Join("testdata", "adduser_3.112ubuntu1.dsc"))
	for _, filename := range tcf2 {
		t.Logf("Package contents of %v:", filename)
		file, err := os.Open(filename)
		if err != nil {
			t.Errorf("cant open file", err)
		}
		cfr := deb.NewControlFileReader(file)
		pkg, err := cfr.Parse()
		if err != nil {
			t.Errorf("cant parse file", err)
		}
		t.Logf("Package contents: %+v", (*pkg)[0])
	}
}
