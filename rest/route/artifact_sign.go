package route

import (
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/gimlet"
)

// artifactSignHandler generates a presigned S3 URL for a signed artifact and
// redirects the caller to it. Authentication is handled via a token
// embedded in the URL rather than session auth, making the URLs curl-friendly.
func artifactSignHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := gimlet.GetVars(r)["task_id"]
		if taskID == "" {
			http.Error(w, "missing task ID", http.StatusBadRequest)
			return
		}

		execStr := r.URL.Query().Get("execution")
		if execStr == "" {
			http.Error(w, "missing execution parameter", http.StatusBadRequest)
			return
		}
		execution, err := strconv.Atoi(execStr)
		if err != nil || execution < 0 {
			http.Error(w, "execution must be a non-negative integer", http.StatusBadRequest)
			return
		}

		fileName := r.URL.Query().Get("name")
		if fileName == "" {
			http.Error(w, "missing name parameter", http.StatusBadRequest)
			return
		}

		token := r.URL.Query().Get("token")
		expiryStr := r.URL.Query().Get("exp")
		if token == "" || expiryStr == "" {
			http.Error(w, "missing token or exp parameter", http.StatusUnauthorized)
			return
		}

		appSecret := []byte(evergreen.GetEnvironment().Settings().ArtifactSignSecret)
		if !artifact.ValidateSignToken(appSecret, taskID, execution, fileName, token, expiryStr) {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		entries, err := artifact.FindAll(ctx, artifact.ByTaskIdAndExecution(taskID, execution))
		if err != nil {
			http.Error(w, "finding artifact entries", http.StatusInternalServerError)
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
			http.Error(w, "artifact file not found", http.StatusNotFound)
			return
		}
		if found.Visibility != artifact.Signed {
			http.Error(w, "artifact is not a signed file", http.StatusBadRequest)
			return
		}

		presignedURL, err := artifact.PresignFile(ctx, *found)
		if err != nil {
			http.Error(w, "presigning artifact URL", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, presignedURL, http.StatusTemporaryRedirect)
	}
}
