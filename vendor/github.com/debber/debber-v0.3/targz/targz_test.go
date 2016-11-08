package targz_test

import (
	"archive/tar"
	"fmt"
	"github.com/debber/debber-v0.3/targz"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// change this to true for generating an archive on the Filesystem
var (
	filename = filepath.Join("testdata", "tmp.tar.gz")
)

func Test_fs(t *testing.T) {
	// Create a buffer to write our archive to.
	wtr := writer()

	// Create a new ar archive.
	tgzw := targz.NewWriter(wtr)

	// Add some files to the archive.
	var files = []struct {
		Name, Body string
	}{
		{"readme.txt", "This archive contains some text files."},
		{"gopher.txt", "Gopher names:\nGeorge\nGeoffrey\nGonzo"},
		{"todo.txt", "Get animal handling licence."},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		if err := tgzw.WriteHeader(hdr); err != nil {
			log.Fatalln(err)
		}
		if _, err := tgzw.Write([]byte(file.Body)); err != nil {
			log.Fatalln(err)
		}
	}
	// Make sure to check the error on Close.
	if err := tgzw.Close(); err != nil {
		log.Fatalln(err)
	}
	rdr := reader(wtr)
	tgzr, err := targz.NewReader(rdr)
	if err != nil {
		log.Fatalln(err)
	}

	// Iterate through the files in the archive.
	for {
		hdr, err := tgzr.Next()
		if err == io.EOF {
			// end of ar archive
			break
		}
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("Contents of %s:\n", hdr.Name)
		if _, err := io.Copy(os.Stdout, tgzr); err != nil {
			log.Fatalln(err)
		}

		fmt.Println()
	}

	// Output:
	// Contents of readme.txt:
	// This archive contains some text files.
	// Contents of gopher.txt:
	// Gopher names:
	// George
	// Geoffrey
	// Gonzo
	// Contents of todo.txt:
	// Get animal handling licence.
}

func reader(w io.Writer) io.Reader {
	fi := w.(*os.File)
	err := fi.Close()
	if err != nil {
		log.Fatalln(err)
	}

	r, err := os.Open(filename)
	if err != nil {
		log.Fatalln(err)
	}
	return r
}

func writer() io.Writer {
	fi, err := os.Create(filename)
	if err != nil {
		log.Fatalln(err)
	}
	return fi

}
