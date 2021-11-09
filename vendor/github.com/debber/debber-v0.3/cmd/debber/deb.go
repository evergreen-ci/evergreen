package main

import (
	"flag"
	"fmt"
	"github.com/debber/debber-v0.3/targz"
	"github.com/laher/argo/ar"
	"io"
	"log"
	"os"
)

func debControl(input []string) {
	args := parseFlagsDeb(input)
	for _, debFile := range args {
		rdr, err := os.Open(debFile)
		if err != nil {
			log.Fatalf("%v", err)
		}
		log.Printf("File: %+v", debFile)
		err = debExtractFileL2(rdr, "control.tar.gz", "control", os.Stdout)
		if err != nil {
			log.Fatalf("%v", err)
		}
	}
}

func debContents(input []string) {
	args := parseFlagsDeb(input)
	for _, debFile := range args {
		rdr, err := os.Open(debFile)
		if err != nil {
			log.Fatalf("%v", err)
		}
		log.Printf("File: %+v", debFile)
		files, err := debGetContents(rdr, "data.tar.gz")
		if err != nil {
			log.Fatalf("%v", err)
		}
		for _, file := range files {
			log.Printf("%s", file)
		}
	}

}

//debGetContents just lists the contents of a tar.gz file within the archive
func debGetContents(rdr io.Reader, topLevelFilename string) ([]string, error) {
	ret := []string{}
	fileNotFound := true
	arr, err := ar.NewReader(rdr)
	if err != nil {
		return nil, err
	}
	for {
		hdr, err := arr.Next()
		if err == io.EOF {
			// end of ar archive
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == topLevelFilename {
			fileNotFound = false
			tgzr, err := targz.NewReader(arr)
			if err != nil {
				e := tgzr.Close()
				if e != nil {
					//log it here?
				}
				return nil, err
			}
			for {
				thdr, err := tgzr.Next()
				if err == io.EOF {
					// end of tar.gz archive
					break
				}
				if err != nil {
					e := tgzr.Close()
					if e != nil {
						//log it here?
					}
					return nil, err
				}
				ret = append(ret, thdr.Name)
			}
		}
	}
	if fileNotFound {
		return nil, fmt.Errorf("File not found")
	}
	return ret, nil
}

func debContentsDebian(input []string) {
	args := parseFlagsDeb(input)
	for _, debFile := range args {
		rdr, err := os.Open(debFile)
		if err != nil {
			log.Fatalf("%v", err)
		}
		log.Printf("File: %+v", debFile)
		files, err := debGetContents(rdr, "control.tar.gz")
		if err != nil {
			log.Fatalf("%v", err)
		}
		for _, file := range files {
			log.Printf("%s", file)
		}
	}

}

func parseFlagsDeb(input []string) []string {
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	err := fs.Parse(input)
	if err != nil {
		log.Fatalf("%v", err)
	}
	args := fs.Args()
	if len(args) < 1 {
		log.Fatalf("File not specified")
	}
	return args
}

//debExtractFileL2 extracts a file from a tar.gz within the archive.
func debExtractFileL2(rdr io.Reader, topLevelFilename string, secondLevelFilename string, destination io.Writer) error {
	arr, err := ar.NewReader(rdr)
	if err != nil {
		return err
	}
	for {
		hdr, err := arr.Next()
		if err == io.EOF {
			// end of ar archive
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == topLevelFilename {
			tgzr, err := targz.NewReader(arr)
			if err != nil {
				tgzr.Close()
				return err
			}
			for {
				thdr, err := tgzr.Next()
				if err == io.EOF {
					// end of tar.gz archive
					break
				}
				if err != nil {
					tgzr.Close()
					return err
				}
				if thdr.Name == secondLevelFilename {
					_, err = io.Copy(destination, tgzr)
					tgzr.Close()
					return nil
				}
				// else skip this file
				log.Printf("File %s", thdr.Name)
			}
			tgzr.Close()
		}
	}
	return fmt.Errorf("File not found")
}
