package artifact

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const Collection = "artifact_files"

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
//
//	Key = Human-readable name for file
//	Value = link for the file
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
	// AWSKey is the key with which the file was uploaded to S3.
	AWSKey string `json:"aws_key,omitempty" bson:"aws_key,omitempty"`
	// AWSSecret is the secret with which the file was uploaded to S3.
	AWSSecret string `json:"aws_secret,omitempty" bson:"aws_secret,omitempty"`
	// AWSRoleARN is the role ARN with which the file was uploaded to S3.
	AWSRoleARN string `json:"aws_role_arn,omitempty" bson:"aws_role_arn,omitempty"`
	// ExternalID is the external ID with which the file was uploaded to S3.
	ExternalID string `json:"external_id,omitempty" bson:"external_id,omitempty"`
	// Bucket is the aws bucket in which the file is stored.
	Bucket string `json:"bucket,omitempty" bson:"bucket,omitempty"`
	// FileKey is the path to the file in the bucket.
	FileKey string `json:"filekey,omitempty" bson:"filekey,omitempty"`
	// ContentType is the content type of the file.
	ContentType string `json:"content_type" bson:"content_type"`
	// FileSize is the size of the file in bytes.
	FileSize int64 `json:"file_size,omitempty" bson:"file_size,omitempty"`
	// PutRequests is the number of S3 PUT requests made to upload this file.
	PutRequests int `json:"put_requests,omitempty" bson:"put_requests,omitempty"`
	// PutCost is the calculated S3 PUT request cost for uploading this file.
	PutCost float64 `json:"put_cost,omitempty" bson:"put_cost,omitempty"`
}

func (f *File) validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.ErrorfWhen(f.Bucket == "", "bucket is required")
	catcher.ErrorfWhen(f.FileKey == "", "file key is required")

	return catcher.Resolve()
}

// StripHiddenFiles is a helper for only showing users the files they are
// allowed to see. It also pre-signs file URLs.
func StripHiddenFiles(ctx context.Context, files []File, hasUser bool) ([]File, error) {
	publicFiles := []File{}
	for _, file := range files {
		switch {
		case file.Visibility == None:
			continue
		case (file.Visibility == Private || file.Visibility == Signed) && !hasUser:
			continue
		case file.Visibility == Signed && hasUser:
			link, err := presignFile(ctx, file)
			if err != nil {
				return nil, errors.Wrapf(err, "presigning url for file '%s'", file.Name)
			}
			file.Link = link
			publicFiles = append(publicFiles, file)
		default:
			publicFiles = append(publicFiles, file)
		}
	}
	return publicFiles, nil
}

func presignFile(ctx context.Context, file File) (string, error) {
	if err := file.validate(); err != nil {
		return "", errors.Wrap(err, "file validation failed")
	}

	if file.AWSRoleARN != "" {
		file.AWSKey = ""
		file.AWSSecret = ""
	}

	var externalID *string
	if file.ExternalID != "" {
		externalID = &file.ExternalID
	}

	requestParams := pail.PreSignRequestParams{
		Bucket:                file.Bucket,
		FileKey:               file.FileKey,
		SignatureExpiryWindow: evergreen.PresignMinimumValidTime,
		AWSKey:                file.AWSKey,
		AWSSecret:             file.AWSSecret,
		AWSRoleARN:            file.AWSRoleARN,
		ExternalID:            externalID,
	}
	return pail.PreSign(ctx, requestParams)
}

func GetAllArtifacts(ctx context.Context, tasks []TaskIDAndExecution) ([]File, error) {
	artifacts, err := FindAll(ctx, ByTaskIdsAndExecutions(tasks))
	if err != nil {
		return nil, errors.Wrap(err, "finding artifact files for task")
	}
	if artifacts == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.TaskID)
		}
		artifacts, err = FindAll(ctx, ByTaskIds(taskIds))
		if err != nil {
			return nil, errors.Wrap(err, "finding artifact files for task without execution number")
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

// EscapeFiles escapes the base of the file link to avoid issues opening links
// with special characters in the UI. Note that it will not escape path segments
// other than the base (i.e. the last one).
// For example, "url.com/something+another/file#1.tar.gz" will be escaped to "url.com/something+another/file%231.tar.gz".
func EscapeFiles(files []File) []File {
	var escapedFiles []File
	for _, file := range files {
		file.Link = escapeFile(file.Link)
		escapedFiles = append(escapedFiles, file)
	}
	return escapedFiles
}

func escapeFile(path string) string {
	base := filepath.Base(path)
	i := strings.LastIndex(path, base)
	if i < 0 {
		return path
	}
	if looksAlreadyEscaped(base) {
		// If the artifact provides a link that already appears to have an
		// escaped base segment, skip escaping the file name to avoid
		// double-escaping.
		return path
	}

	return path[:i] + strings.Replace(path[i:], base, url.QueryEscape(base), 1)
}

// looksAlreadyEscaped checks whether a given URL path segment appears to be
// already escaped. This can happen if the user manually escapes the URL.
func looksAlreadyEscaped(pathSegment string) bool {
	if !strings.Contains(pathSegment, "%") {
		return false
	}

	unescaped, err := url.QueryUnescape(pathSegment)
	if err != nil {
		// Attempting to unescape didn't work, which means pathSegment is not a
		// valid escaped string. For example, unescaping could error if the URL
		// happens to contain a percent sign in it but isn't actually escaped.
		return false
	}

	// If unescaping and re-escaping gives the original pathSegment, then the
	// pathSegment has already been properly escaped.
	return unescaped != pathSegment && url.QueryEscape(unescaped) == pathSegment
}
