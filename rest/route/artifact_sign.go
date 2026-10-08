package route

import (
	"net/http"
	"strconv"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// artifactSignHandler generates a presigned S3 URL for a signed artifact and
// redirects the caller to it. Authentication is handled via a token
// embedded in the URL rather than session auth, making the URLs curl-friendly.
func artifactSignHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), evergreen.ArtifactSignOtelSpanName)
		defer span.End()

		writeErr := func(w http.ResponseWriter, code int, msg string) {
			span.SetAttributes(attribute.Int(evergreen.ArtifactSignStatusCodeOtelAttribute, code))
			span.SetStatus(codes.Error, msg)
			http.Error(w, msg, code)
		}

		taskID := gimlet.GetVars(r)["task_id"]
		if taskID == "" {
			writeErr(w, http.StatusBadRequest, "missing task ID")
			return
		}
		span.SetAttributes(attribute.String(evergreen.TaskIDOtelAttribute, taskID))

		execStr := r.URL.Query().Get("execution")
		if execStr == "" {
			writeErr(w, http.StatusBadRequest, "missing execution parameter")
			return
		}
		execution, err := strconv.Atoi(execStr)
		if err != nil || execution < 0 {
			writeErr(w, http.StatusBadRequest, "execution must be a non-negative integer")
			return
		}
		span.SetAttributes(attribute.Int(evergreen.TaskExecutionOtelAttribute, execution))

		fileName := r.URL.Query().Get("name")
		if fileName == "" {
			writeErr(w, http.StatusBadRequest, "missing name parameter")
			return
		}

		token := r.URL.Query().Get("token")
		expiryStr := r.URL.Query().Get("exp")
		if token == "" || expiryStr == "" {
			writeErr(w, http.StatusUnauthorized, "missing token or exp parameter")
			return
		}

		appSecret := []byte(evergreen.GetEnvironment().Settings().ArtifactSignSecret)
		if len(appSecret) == 0 {
			writeErr(w, http.StatusInternalServerError, "artifact signing is not configured")
			return
		}
		if !artifact.ValidateSignToken(appSecret, taskID, execution, fileName, token, expiryStr) {
			writeErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		entries, err := artifact.FindAll(ctx, artifact.ByTaskIdAndExecution(taskID, execution))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "finding artifact entries")
			return
		}

		var found *artifact.File
		for _, entry := range entries {
			for i, file := range entry.Files {
				if file.Name == fileName {
					found = &entry.Files[i]
					break
				}
			}
			if found != nil {
				break
			}
		}
		if found == nil {
			writeErr(w, http.StatusNotFound, "artifact file not found")
			return
		}
		if found.Visibility != artifact.Signed {
			writeErr(w, http.StatusBadRequest, "artifact is not a signed file")
			return
		}

		resolver := model.NewArtifactCredentialResolver(taskID)

		// A presigned S3 URL is only valid for the method it was signed for, so
		// redirecting a HEAD request to a presigned GET URL would fail. Answer
		// HEAD requests directly with the object's metadata instead.
		if r.Method == http.MethodHead {
			head, err := artifact.HeadFile(ctx, *found, resolver)
			if err != nil {
				var notFound *s3types.NotFound
				if errors.As(err, &notFound) {
					writeErr(w, http.StatusNotFound, "artifact object not found")
					return
				}
				writeErr(w, http.StatusInternalServerError, "fetching artifact metadata")
				return
			}
			writeHeadObjectHeaders(w, head)
			w.WriteHeader(http.StatusOK)
			return
		}

		presignedURL, err := artifact.PresignFile(ctx, *found, resolver)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "presigning artifact URL")
			return
		}

		http.Redirect(w, r, presignedURL, http.StatusTemporaryRedirect)
	}
}

func writeHeadObjectHeaders(w http.ResponseWriter, head *awss3.HeadObjectOutput) {
	if head.ContentLength != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(*head.ContentLength, 10))
	}
	if head.ContentType != nil {
		w.Header().Set("Content-Type", *head.ContentType)
	}
	if head.ETag != nil {
		w.Header().Set("ETag", *head.ETag)
	}
	if head.LastModified != nil {
		w.Header().Set("Last-Modified", head.LastModified.UTC().Format(http.TimeFormat))
	}
}
