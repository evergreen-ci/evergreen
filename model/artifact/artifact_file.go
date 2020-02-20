package artifact

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const Collection = "artifact_files"
const PresignExpireTime = 15 * time.Minute

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
	TaskId          string `json:"task" bson:"task"`
	TaskDisplayName string `json:"task_name" bson:"task_name"`
	BuildId         string `json:"build" bson:"build"`
	Files           []File `json:"files" bson:"files"`
	Execution       int    `json:"execution" bson:"execution"`
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
	//FileKey is the path to the file in the buckt
	FileKey string `json:"filekey,omitempty" bson:"filekey,omitempty"`
}

// stripHiddenFiles is a helper for only showing users the files they are allowed to see.
func StripHiddenFiles(files []File, hasUser bool) []File {
	publicFiles := []File{}
	for _, file := range files {
		switch {
		case file.Visibility == None:
			continue
		case file.Visibility == Private && hasUser == false:
			continue
		default:
			publicFiles = append(publicFiles, file)
		}
	}
	return publicFiles
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
	catcher := grip.NewBasicCatcher()
	for _, artifact := range artifacts {
		for i := range artifact.Files {
			if artifact.Files[i].Visibility == "signed" {
				if artifact.Files[i].AwsSecret == "" || artifact.Files[i].AwsKey == "" || artifact.Files[i].Bucket == "" || artifact.Files[i].FileKey == "" {
					err = errors.New("error presigning the url for artifact")
					catcher.Add(err)
				}

				sess, err := session.NewSession(&aws.Config{
					Region: aws.String("us-east-1"),
					Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
						AccessKeyID:     artifact.Files[i].AwsKey,
						SecretAccessKey: artifact.Files[i].AwsSecret,
					}),
				})
				if err != nil {
					log.Fatalf("problem creating session: %s", err.Error())
				}
				svc := s3.New(sess)

				req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
					Bucket: aws.String(artifact.Files[i].Bucket),
					Key:    aws.String(artifact.Files[i].FileKey),
				})

				urlStr, err := req.Presign(PresignExpireTime)
				if err != nil {
					log.Fatalf("problem signing request: %s", err.Error())
				}
				artifact.Files[i].Link = urlStr
			}
		}
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
