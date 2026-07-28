package route

import (
	"context"
	"net/http"
	"slices"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/ratelimit"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/{user_id}/rate_limit

type userRateLimitGetHandler struct {
	env    evergreen.Environment
	userID string
}

func makeUserRateLimitGetHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &userRateLimitGetHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a user's REST rate limit status
//	@Description	Get the caller's current REST rate limit status. Callers may only check their own rate limit.
//	@Tags			users
//	@Router			/users/{user_id}/rate_limit [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id	path		string						true	"User ID"
//	@Success		200		{object}	model.APIRateLimitStatus
func (h *userRateLimitGetHandler) Factory() gimlet.RouteHandler {
	return &userRateLimitGetHandler{env: h.env}
}

func (h *userRateLimitGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.userID = gimlet.GetVars(r)["user_id"]
	return nil
}

func (h *userRateLimitGetHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	if u.Username() != h.userID {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "users may only check their own rate limit",
		})
	}

	cfg := h.env.Settings().RateLimit
	perHour, burst := limitsFor(&cfg, evergreen.RateLimitSurfaceREST, u.OnlyAPI)
	if slices.Contains(cfg.ElevatedUserIDs, u.Username()) {
		perHour *= 2
		burst *= 2
	}

	limiter, err := ratelimit.NewRateLimiter(h.env.RedisClient())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "initializing rate limiter"))
	}
	// Use peek method to check the user's rate limit without consuming any tokens.
	result, err := limiter.Peek(ctx, u.Username(), evergreen.RateLimitSurfaceREST, perHour, burst)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking rate limit"))
	}

	status := &restmodel.APIRateLimitStatus{}
	status.BuildFromService(result)
	return gimlet.NewJSONResponse(status)
}
