package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"os"
	"time"
)

const (
	b  = 1024
	kb = b * 1024
	mb = kb * 1024
	gb = mb * 1024
	tb = gb * 1024
)

type PurgeOptions struct {
	AccessKey    string  `short:"k" long:"accessKey" description:"S3 access key" required:"true"`
	SecretKey    string  `short:"s" long:"secretKey" description:"S3 secret key" required:"true"`
	TargetBucket string  `short:"b" long:"bucket" description:"S3 target bucket" required:"true"`
	Execute      bool    `short:"y" long:"execute" description:"Execution mode"`
	Threshold    float64 `short:"d" long:"days" description:"Number of days to retain files" default:"180"`
}

type purgeData struct {
	// handle to S3 bucket
	bucket *s3.Bucket
	// contents of an S3 bucket
	contents []s3.Key
	// number of days to retain files
	threshold float64
	// reference time for purge operation
	beginTime time.Time
}

var (
	skippedFilesSize = int64(0)
	recentFilesSize  = int64(0)
	deletedFilesSize = int64(0)
)

var (
	options PurgeOptions
)

func main() {
	if _, err := flags.NewParser(&options, flags.Default).Parse(); err != nil {
		fmt.Println("This script will delete all files in the given S3 bucket that were last modified more '--days' days ago.\n--accessKey, --secretKey and --bucket required.\nPass '--execute' for an actual run.")
		os.Exit(1)
	}

	auth := aws.Auth{
		AccessKey: options.AccessKey,
		SecretKey: options.SecretKey,
	}
	bucket := s3.New(auth, aws.USEast).Bucket(options.TargetBucket)

	marker := ""
	numPurgeOps := 0

	data := &purgeData{
		bucket:    bucket,
		beginTime: time.Now(),
		threshold: options.Threshold * 24.0,
	}
	// iterate through all items in the target bucket and
	// perform purge as needed
	for {
		resp, err := data.bucket.List("", "", marker, 0)
		if err != nil {
			fmt.Printf(fmt.Sprintf("list error: %v\n", err))
			return
		}
		if len(resp.Contents) == 0 {
			fmt.Println("no content returned")
			break
		}
		data.contents = resp.Contents
		executePurge(data)
		lastKey := resp.Contents[len(resp.Contents)-1].Key
		if resp.IsTruncated {
			marker = resp.NextMarker
			if marker == "" {
				marker = lastKey
			}
		} else {
			break
		}
		numPurgeOps += 1

		// throttle our requests?
		if numPurgeOps%100 == 0 {
			fmt.Printf("num purges: %v; marker: %v\n", numPurgeOps, marker)
		}
	}

	fmt.Printf("recent files size %v\n", readableSize(recentFilesSize))
	fmt.Printf("deleted files size %v\n", readableSize(deletedFilesSize))
	fmt.Printf("skipped files size %v\n", readableSize(skippedFilesSize))
	fmt.Println("done!")
}

func readableSize(val int64) string {
	if val < b {
		return fmt.Sprintf("%vB", val)
	} else if val >= b && val < kb {
		return fmt.Sprintf("%.2fKB", float64(val)/b)
	} else if val >= kb && val < mb {
		return fmt.Sprintf("%.2fMB", float64(val)/kb)
	} else if val >= mb && val < gb {
		return fmt.Sprintf("%.2fGB", float64(val)/mb)
	} else if val >= gb && val < tb {
		return fmt.Sprintf("%.2fTB", float64(val)/gb)
	}
	return fmt.Sprintf("%.2fPB", float64(val)/tb)
}

func executePurge(data *purgeData) {
	paths := make([]string, 0, 1000)
	for _, obj := range data.contents {
		lastModified, err := time.Parse(time.RFC3339, obj.LastModified)
		if err != nil {
			fmt.Printf("error parsing object %#v: %v\n", obj, err)
			continue
		}
		if isFolder(obj.Key) {
			fmt.Printf("skipping folder %v (%v)\n", obj.Key, readableSize(obj.Size))
			continue
		}

		// if item has expired, delete it
		if data.beginTime.Sub(lastModified).Hours() <= data.threshold {
			fmt.Printf("keeping (recent): %v (%v)\n", obj.Key, readableSize(obj.Size))
			recentFilesSize += obj.Size
		} else {
			fmt.Printf("will delete: %v (%v)\n", obj.Key, readableSize(obj.Size))
			if options.Execute {
				paths = append(paths, obj.Key)
				deletedFilesSize += obj.Size
				if len(paths) == 1000 {
					if err := data.bucket.MultiDel(paths); err != nil {
						fmt.Printf("error deleting files: %v", err)
					}
					fmt.Printf("deleted batch\n")
					paths = paths[:0]
				}
			}
		}
	}
	if len(paths) != 0 {
		if err := data.bucket.MultiDel(paths); err != nil {
			fmt.Printf("error deleting files: %v", err)
		}
		fmt.Printf("deleted batch\n")
	}
}

func isFolder(path string) bool {
	return path[len(path)-1] == '/'
}
