/*
   Copyright 2013 Am Laher

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package debgen

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TarWriterHelper makes directories for you, and has some simple functions for adding files & folders.
// This can be used in conjunction with gzip compression (or others)
type TarWriterHelper struct {
	Tw *tar.Writer
	DirsMade []string
}
func NewTarWriterHelper(tw *tar.Writer) *TarWriterHelper {
	twh := &TarWriterHelper{tw, []string{}}
	return twh
}

// TarHeader is a factory for a tar header. Fixes slashes, populates ModTime
func TarHeader(path string, datalen int64, mode int64) *tar.Header {
	h := new(tar.Header)
	//slash-only paths
	h.Name = strings.Replace(path, "\\", "/", -1)
	if strings.HasPrefix(h.Name, "/") {
		h.Name = "." + h.Name
	}
	h.Size = datalen
	h.Mode = mode
	h.ModTime = time.Now()
	return h
}

// TarAddFile adds a file from the file system
// This is just a helper function
// TODO: directories
func (twh *TarWriterHelper) AddFile(sourceFile, destName string) error {
	err := twh.AddParentDirs(destName)
	if err != nil {
		return err
	}
	fi, err := os.Open(sourceFile)
	defer fi.Close()
	if err != nil {
		return err
	}
	finf, err := fi.Stat()
	if err != nil {
		return err
	}

	//recurse as necessary
	if finf.IsDir() {
		return fmt.Errorf("Can't add a directory '%s' using TarAddFile. See AddFileOrDir", sourceFile)
	}
	err = twh.Tw.WriteHeader(TarHeader(destName, finf.Size(), int64(finf.Mode())))
	if err != nil {
		log.Printf("Can't add a header for '%s' using TarAddFile. Error: %v", destName, err)
		return err
	}
	_, err = io.Copy(twh.Tw, fi)
	if err != nil {
		return err
	}
	return nil
}

func (twh *TarWriterHelper) AddFileOrDir(sourceFile, destName string) error {
	finf, err := os.Stat(sourceFile)
	if err != nil {
		return err
	}
	//recurse as necessary
	if finf.IsDir() {
		err = twh.AddParentDirs(destName)
		if err != nil {
			return err
		}
		mode := int64(0755 | 040000)
		th := TarHeader(destName, 0, mode)
		th.Typeflag = tar.TypeDir
		err = twh.Tw.WriteHeader(th)
		if err != nil {
			log.Printf("Can't add a header for '%s' using AddFileOrDir. Error: %v", destName, err)
			return err
		}
		err = filepath.Walk(sourceFile, func(path string, info os.FileInfo, err2 error) error {
			if info != nil && !info.IsDir() {
				rel, err := filepath.Rel(sourceFile, path)
				if err == nil {
					return twh.AddFile(rel, path)
				}
				return err
			}
			return nil
		})
		// return now
		return err
	}

	return twh.AddFile(sourceFile, destName)
}
//AddParentDirs adds the necessary dirs for debian-friendly tar archives
func (twh *TarWriterHelper) AddParentDirs(filename string) error {
	parentDirParts := strings.Split(filename, "/")
	acc := ""
	for _, pdp := range parentDirParts[0 : len(parentDirParts)-1] {
		acc += pdp + "/"

		if acc == "/" {
		} else {
			alreadyMade := false
			for _, dirMade := range twh.DirsMade {
				if dirMade == acc {
					alreadyMade = true
				}
			}
			if !alreadyMade {
				mode := int64(0755 | 040000)
				th := TarHeader(acc, 0, mode)
				th.Typeflag = tar.TypeDir
				err := twh.Tw.WriteHeader(th)
				if err != nil {
					log.Printf("Can't add a header for '%s' using AddParentDirs.", acc)
					return err
				}
				twh.DirsMade = append(twh.DirsMade, acc)
			}
		}
	}
	return nil
}

// AddFiles adds resources from file system.
// The key should be the destination filename. Value is the local filesystem path
func (twh *TarWriterHelper) AddFiles(resources map[string]string) error {
	if resources != nil {
		for name, localPath := range resources {
			err := twh.AddFile(localPath, name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
// AddFiles adds resources from file system.
// The key should be the destination filename/dirname. Value is the local filesystem path
func (twh *TarWriterHelper) AddFilesOrDirs(resources map[string]string) error {
	if resources != nil {
		for name, localPath := range resources {
			err := twh.AddFileOrDir(localPath, name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// AddBytes adds a file by bytes with a given path
func (twh *TarWriterHelper) AddBytes(bytes []byte, destName string, mode int64) error {
	err := twh.AddParentDirs(destName)
	if err != nil {
		return err
	}
	err = twh.Tw.WriteHeader(TarHeader(destName, int64(len(bytes)), mode))
	if err != nil {
		log.Printf("Can't add a header for '%s' using AddBytes.", destName)
		return err
	}
	_, err = twh.Tw.Write(bytes)
	if err != nil {
		return err
	}
	return nil
}
