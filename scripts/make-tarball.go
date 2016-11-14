/*
Archive

Provides a single "MakeTarball" function to create tar (tar.gz)
archives. Uses go libraries rather than calling out to GNU tar or
similar.  Is more cross platform and makes it easy to prefix all
contents inside of a directory that does not exist in the source.
*/
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// inspired by https://gist.github.com/jonmorehouse/9060515

type archiveWorkUnit struct {
	path string
	stat os.FileInfo
}

func getContents(paths []string, exclusions []string) <-chan archiveWorkUnit {
	var matchers []*regexp.Regexp
	for _, pattern := range exclusions {
		matchers = append(matchers, regexp.MustCompile(pattern))
	}

	output := make(chan archiveWorkUnit)

	go func() {
		for _, path := range paths {
			err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				for _, exclude := range matchers {
					if exclude.MatchString(p) {
						return nil
					}
				}

				output <- archiveWorkUnit{
					path: p,
					stat: info,
				}
				return nil
			})

			if err != nil {
				panic(fmt.Sprintf("caught error walking file system: %+v", err))
			}
		}
		close(output)
	}()

	return output
}

func addFile(tw *tar.Writer, prefix string, unit archiveWorkUnit) error {
	file, err := os.Open(unit.path)
	if err != nil {
		return err
	}
	defer file.Close()
	// now lets create the header as needed for this file within the tarball
	header := new(tar.Header)
	header.Name = filepath.Join(prefix, unit.path)
	header.Size = unit.stat.Size()
	header.Mode = int64(unit.stat.Mode())
	header.ModTime = unit.stat.ModTime()
	// write the header to the tarball archive
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	// copy the file data to the tarball
	if _, err := io.Copy(tw, file); err != nil {
		return err
	}

	// fmt.Println("DEBUG: added %s to archive", header.Name)
	return nil
}

func makeTarball(fileName, prefix string, paths []string, exclude []string) error {
	// set up the output file
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("problem creating file %s: %v", fileName, err)
	}
	defer file.Close()

	// set up the  gzip writer
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	fmt.Println("creating archive:", fileName)

	for unit := range getContents(paths, exclude) {
		err := addFile(tw, prefix, unit)

		if err != nil {
			return fmt.Errorf("error adding path: %s [%+v]: %v",
				unit.path, unit, err)
		}
	}

	return nil
}

type stringSlice []string

func (i *stringSlice) Set(v string) error { *i = append(*i, v); return nil }
func (i *stringSlice) String() string     { return strings.Join([]string(*i), ", ") }

func main() {
	var (
		name     string
		prefix   string
		items    stringSlice
		excludes stringSlice
	)

	flag.Var(&items, "item", "specify item to add to the archive")
	flag.Var(&excludes, "exclude", "regular expressions to exclude files")
	flag.StringVar(&name, "name", "archive.tar.gz", "full path to the archive")
	flag.StringVar(&prefix, "prefix", "", "prefix of path within the archive")
	flag.Parse()

	if err := makeTarball(name, prefix, items, excludes); err != nil {
		fmt.Printf("ERROR: %+v\n", err)
		os.Exit(1)
	}

	fmt.Println("created archive:", name)
}
