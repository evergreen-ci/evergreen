package artifact

import (
	"time"

	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

const Collection = "artifact_files"
const PresignExpireTime = 24 * time.Hour

const (
	// strings for setting visibility
	Public  = "public"
	Private = "private"
	None    = "none"
	Signed  = "signed"
)

var ValidVisibilities = []string{Public, Private, None, Signed, ""}

// Entry stores groups of names and links (not content!) for
// files uploaded to the api server by a running agent. These links could
// be for build or task-relevant files (things like extra results,
// test coverage, etc.)
type Entry struct {
	TaskId          string    `json:"task" bson:"task"`
	TaskDisplayName string    `json:"task_name" bson:"task_name"`
	BuildId         string    `json:"build" bson:"build"`
	Files           []File    `json:"files" bson:"files"`
	Execution       int       `json:"execution" bson:"execution"`
	CreateTime      time.Time `json:"create_time" bson:"create_time"`
}

// Params stores file entries as key-value pairs, for easy parameter parsing.
//  Key = Human-readable name for file
//  Value = link for the file
type Params map[string]string

// File is a pairing of name and link for easy storage/display
type File struct {
	// Name is a human-readable name for the file being linked, e.g. "Coverage Report"
	Name string `json:"name" bson:"name"`
	// Link is the link to the file, e.g. "http://fileserver/coverage.html"
	Link string `json:"link" bson:"link"`
	// Visibility determines who can see the file in the UI
	Visibility string `json:"visibility" bson:"visibility"`
	// When true, these artifacts are excluded from reproduction
	IgnoreForFetch bool `bson:"fetch_ignore,omitempty" json:"ignore_for_fetch"`
	//AwsKey is the key with which the file was uploaded to s3
	AwsKey string `json:"aws_key,omitempty" bson:"aws_key,omitempty"`
	//AwsSercret is the secret with which the file was uploaded to s3
	AwsSecret string `json:"aws_secret,omitempty" bson:"aws_secret,omitempty"`
	//Bucket is the aws bucket in which the file is stored
	Bucket string `json:"bucket,omitempty" bson:"bucket,omitempty"`
	//FileKey is the path to the file in the bucket
	FileKey string `json:"filekey,omitempty" bson:"filekey,omitempty"`
}

// StripHiddenFiles is a helper for only showing users the files they are allowed to see.
func StripHiddenFiles(files []File, hasUser bool) ([]File, error) {
	publicFiles := []File{}
	for _, file := range files {
		switch {
		case file.Visibility == None:
			continue
		case (file.Visibility == Private || file.Visibility == Signed) && hasUser == false:
			continue
		case file.Visibility == Signed && hasUser:
			if !file.ContainsSigningParams() {
				return nil, errors.Errorf("error presigning the url for %s, awsSecret, awsKey, bucket, or filekey missing", file.Name)
			}
			requestParams := thirdparty.RequestParams{
				Bucket:    file.Bucket,
				FileKey:   file.FileKey,
				AwsKey:    file.AwsKey,
				AwsSecret: file.AwsSecret,
			}
			urlStr, err := thirdparty.PreSign(requestParams)
			if err != nil {
				return nil, errors.Wrap(err, "problem presigning url")
			}
			file.Link = urlStr
			publicFiles = append(publicFiles, file)
		default:
			publicFiles = append(publicFiles, file)
		}
	}
	return publicFiles, nil
}

//ContainsSigningParams returns true if all the params needed for
//presigning a url are present
func (f *File) ContainsSigningParams() bool {
	return !(f.AwsSecret == "" || f.AwsKey == "" || f.Bucket == "" || f.FileKey == "")
}

func GetAllArtifacts(tasks []TaskIDAndExecution) ([]File, error) {
	artifacts, err := FindAll(ByTaskIdsAndExecutions(tasks))
	if err != nil {
		return nil, errors.Wrap(err, "error finding artifact files for task")
	}
	if artifacts == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.TaskID)
		}
		artifacts, err = FindAll(ByTaskIds(taskIds))
		if err != nil {
			return nil, errors.Wrap(err, "error finding artifact files for task without execution number")
		}
		if artifacts == nil {
			return []File{}, nil
		}
	}
	files := []File{}
	for _, artifact := range artifacts {
		files = append(files, artifact.Files...)
	}
	return files, nil
}

// Array turns the parameter map into an array of File structs.
// Deprecated.
func (params Params) Array() []File {
	var files []File
	for name, link := range params {
		files = append(files, File{
			Name: name,
			Link: link,
		})
	}
	return files
}
