package testutil

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mholt/archiver"
	"github.com/mongodb/grip"
)

func AddFileToDirectory(dir string, fileName string, fileContents string) error {
	if format := archiver.MatchingFormat(fileName); format != nil {
		tmpFile, err := ioutil.TempFile(dir, "tmp.txt")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpFile.Name())
		if _, err := tmpFile.Write([]byte(fileContents)); err != nil {
			catcher := grip.NewBasicCatcher()
			catcher.Add(err)
			catcher.Add(tmpFile.Close())
			return catcher.Resolve()
		}
		if err := tmpFile.Close(); err != nil {
			return err
		}

		if err := format.Make(filepath.Join(dir, fileName), []string{tmpFile.Name()}); err != nil {
			return err
		}
		return nil
	}

	file, err := os.Create(filepath.Join(dir, fileName))
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte(fileContents)); err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Add(err)
		catcher.Add(file.Close())
		return catcher.Resolve()
	}
	return file.Close()
}
