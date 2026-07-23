package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type githubPatchIntentProcessingErrorHandler struct {
	intentID string
}

type githubPatchIntentProcessingErrorResponse struct {
	ID              string    `json:"id"`
	IntentType      string    `json:"intent_type"`
	ProcessingError string    `json:"processing_error"`
	CreatedAt       time.Time `json:"created_at"`
	Processed       bool      `json:"processed"`
	ProcessedAt     time.Time `json:"processed_at"`

	BaseRepoName string `json:"base_repo_name,omitempty"`
	BaseBranch   string `json:"base_branch,omitempty"`
	HeadRepoName string `json:"head_repo_name,omitempty"`
	HeadBranch   string `json:"head_branch,omitempty"`
	PRNumber     int    `json:"pr_number,omitempty"`
	HeadHash     string `json:"head_hash,omitempty"`
	BaseHash     string `json:"base_hash,omitempty"`
	MergeBase    string `json:"merge_base,omitempty"`
	User         string `json:"user,omitempty"`
	Title        string `json:"title,omitempty"`
	GitHubPRURL  string `json:"github_pr_url,omitempty"`

	Org            string    `json:"org,omitempty"`
	Repo           string    `json:"repo,omitempty"`
	HeadRef        string    `json:"head_ref,omitempty"`
	HeadCommit     string    `json:"head_commit,omitempty"`
	HeadCommitDate time.Time `json:"head_commit_date,omitempty"`
}

func makeGithubPatchIntentProcessingError() gimlet.RouteHandler {
	return &githubPatchIntentProcessingErrorHandler{}
}

func (h *githubPatchIntentProcessingErrorHandler) Factory() gimlet.RouteHandler {
	return &githubPatchIntentProcessingErrorHandler{}
}

func (h *githubPatchIntentProcessingErrorHandler) Parse(ctx context.Context, r *http.Request) error {
	h.intentID = gimlet.GetVars(r)["intent_id"]
	if h.intentID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "intent ID must not be empty",
		}
	}
	return nil
}

func (h *githubPatchIntentProcessingErrorHandler) Run(ctx context.Context) gimlet.Responder {
	intent, err := patch.FindGitHubIntentProcessingError(ctx, h.intentID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding GitHub patch intent '%s'", h.intentID))
	}
	if intent == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("GitHub patch intent '%s' not found", h.intentID),
		})
	}

	return gimlet.NewJSONResponse(newGithubPatchIntentProcessingErrorResponse(intent))
}

func newGithubPatchIntentProcessingErrorResponse(intent *patch.GitHubIntentProcessingError) githubPatchIntentProcessingErrorResponse {
	resp := githubPatchIntentProcessingErrorResponse{
		ID:              intent.ID,
		IntentType:      intent.IntentType,
		ProcessingError: intent.ProcessingError,
		CreatedAt:       intent.CreatedAt,
		Processed:       intent.Processed,
		ProcessedAt:     intent.ProcessedAt,
		BaseRepoName:    intent.BaseRepoName,
		BaseBranch:      intent.BaseBranch,
		HeadRepoName:    intent.HeadRepoName,
		HeadBranch:      intent.HeadBranch,
		PRNumber:        intent.PRNumber,
		HeadHash:        intent.HeadHash,
		BaseHash:        intent.BaseHash,
		MergeBase:       intent.MergeBase,
		User:            intent.User,
		Title:           intent.Title,
		Org:             intent.Org,
		Repo:            intent.Repo,
		HeadRef:         intent.HeadRef,
		HeadCommit:      intent.HeadCommit,
		HeadCommitDate:  intent.HeadCommitDate,
	}
	if intent.BaseRepoName != "" && intent.PRNumber != 0 {
		resp.GitHubPRURL = fmt.Sprintf("https://github.com/%s/pull/%d", intent.BaseRepoName, intent.PRNumber)
	}
	return resp
}
