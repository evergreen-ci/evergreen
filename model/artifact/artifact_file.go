package artifact

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

const Collection = "artifact_files"

const (
	// strings for setting visibility
	Public  = "public"
	Private = "private"
	None    = "none"
	Signed  = "signed"

	artifactFileAttribute = "evergreen.artifact_file"
)

var (
	fileNameAttribute       = fmt.Sprintf("%s.file_name", artifactFileAttribute)
	bucketAttribute         = fmt.Sprintf("%s.bucket", artifactFileAttribute)
	internalBucketAttribute = fmt.Sprintf("%s.internal_bucket", artifactFileAttribute)
	fileKeyAttribute        = fmt.Sprintf("%s.file_key", artifactFileAttribute)
	errorAttribute          = fmt.Sprintf("%s.error", artifactFileAttribute)
	irsaErrorAttribute      = fmt.Sprintf("%s.irsa_error", artifactFileAttribute)
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
	// AwsKey is the key with which the file was uploaded to S3.
	AwsKey string `json:"aws_key,omitempty" bson:"aws_key,omitempty"`
	// AwsSecret is the secret with which the file was uploaded to S3.
	AwsSecret string `json:"aws_secret,omitempty" bson:"aws_secret,omitempty"`
	// Bucket is the aws bucket in which the file is stored.
	Bucket string `json:"bucket,omitempty" bson:"bucket,omitempty"`
	// FileKey is the path to the file in the bucket.
	FileKey string `json:"filekey,omitempty" bson:"filekey,omitempty"`
	// ContentType is the content type of the file.
	ContentType string `json:"content_type" bson:"content_type"`
}

func (f *File) validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.ErrorfWhen(f.Bucket == "", "bucket is required")
	catcher.ErrorfWhen(f.FileKey == "", "file key is required")

	// Buckets that are not devprod owned require AWS credentials.
	if !isInternalBucket(f.Bucket) {
		catcher.ErrorfWhen(f.AwsKey == "", "AWS key is required")
		catcher.ErrorfWhen(f.AwsSecret == "", "AWS secret is required")
	}

	return catcher.Resolve()
}

// StripHiddenFiles is a helper for only showing users the files they are
// allowed to see. It also pre-signs file URLs.
func StripHiddenFiles(ctx context.Context, files []File, hasUser bool) ([]File, error) {
	ctx, span := tracer.Start(ctx, "strip-hidden-files")
	defer span.End()
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
	ctx, span := tracer.Start(ctx, "presign-file")
	defer span.End()
	isInternalBucket := isInternalBucket(file.Bucket)
	span.SetAttributes(
		attribute.String(fileNameAttribute, file.Name),
		attribute.String(bucketAttribute, file.Bucket),
		attribute.String(fileKeyAttribute, file.FileKey),
		attribute.Bool(internalBucketAttribute, isInternalBucket),
	)
	if err := errors.Wrap(file.validate(), "file validation error"); err != nil {
		span.SetAttributes(attribute.String(errorAttribute, err.Error()))
		return "", err
	}

	// If this bucket is a devprod owned one, we sign the URL
	// with the app's server IRSA credentials (which is used
	// when no credentials are provided). If it fails,
	// we fallback to using the provided credentials.
	if isInternalBucket {
		requestParams := pail.PreSignRequestParams{
			Bucket:                file.Bucket,
			FileKey:               file.FileKey,
			SignatureExpiryWindow: evergreen.PresignMinimumValidTime,
		}
		presignURL, err := pail.PreSign(ctx, requestParams)
		if err = errors.Wrap(err, "presigning internal bucket file"); err != nil {
			span.SetAttributes(attribute.String(errorAttribute, err.Error()))
			return "", err
		}
		if err := verifyPresignURL(ctx, presignURL); err != nil {
			span.SetAttributes(attribute.String(irsaErrorAttribute, err.Error()))
		} else {
			return presignURL, nil
		}
	}

	requestParams := pail.PreSignRequestParams{
		Bucket:                file.Bucket,
		FileKey:               file.FileKey,
		AwsKey:                file.AwsKey,
		AwsSecret:             file.AwsSecret,
		SignatureExpiryWindow: evergreen.PresignMinimumValidTime,
	}
	presignURL, err := pail.PreSign(ctx, requestParams)
	if err = errors.Wrap(err, "presigning file"); err != nil {
		span.SetAttributes(attribute.String(errorAttribute, err.Error()))
		return "", err
	}
	return presignURL, nil
}

func verifyPresignURL(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "creating request to presign URL")
	}

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "making request to presign URL")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("presign URL returned status code '%d'", resp.StatusCode)
	}

	return nil
}

func GetAllArtifacts(tasks []TaskIDAndExecution) ([]File, error) {
	artifacts, err := FindAll(ByTaskIdsAndExecutions(tasks))
	if err != nil {
		return nil, errors.Wrap(err, "finding artifact files for task")
	}
	if artifacts == nil {
		taskIds := []string{}
		for _, t := range tasks {
			taskIds = append(taskIds, t.TaskID)
		}
		artifacts, err = FindAll(ByTaskIds(taskIds))
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

func RotateSecrets(toReplace, replacement string, dryRun bool) (map[TaskIDAndExecution][]string, error) {
	catcher := grip.NewBasicCatcher()
	artifacts, err := FindAll(BySecret(toReplace))
	catcher.Wrap(err, "finding artifact files by secret")
	changes := map[TaskIDAndExecution][]string{}
	for i, artifact := range artifacts {
		for j, file := range artifact.Files {
			if file.AwsSecret == toReplace {
				if !dryRun {
					artifacts[i].Files[j].AwsSecret = replacement
					catcher.Wrapf(artifacts[i].Update(), "updating artifact file info for task '%s', execution %d", artifact.TaskId, artifact.Execution)
				}
				key := TaskIDAndExecution{
					TaskID:    artifact.TaskId,
					Execution: artifact.Execution,
				}
				changes[key] = append(changes[key], file.Name)
			}
		}
	}
	return changes, catcher.Resolve()
}

// EscapeFiles escapes the base of the file link to avoid issues opening links
// with special characters in the UI.
// For example, "url.com/something/file#1.tar.gz" will be escaped to "url.com/something/file%231.tar.gz".
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
	return path[:i] + strings.Replace(path[i:], base, url.QueryEscape(base), 1)
}

// isInternalBucket returns true if the bucket can be accessed by the app server's
// IRSA role.
func isInternalBucket(bucketName string) bool {
	return utility.StringSliceContains(evergreen.GetEnvironment().Settings().Buckets.InternalBuckets, bucketName)
}
