package route

import (
	"context"
	"fmt"
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
//	@Description	Get the caller's current REST rate limit status. Callers may only check their own rate limit. Returns a 503 if rate limiting is disabled globally or for the caller's user type.
//	@Tags			users
//	@Router			/users/{user_id}/rate_limit [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id	path		string	true	"User ID"
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
			Message:    fmt.Sprintf("users may only check their own rate limit %s", u.Username()),
		})
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking rate limit status"))
	}
	if flags.APIRateLimiterDisabled {
		return rateLimitingDisabledResponder()
	}

	cfg := h.env.Settings().RateLimit
	perHour, burst := limitsFor(&cfg, evergreen.RateLimitSurfaceREST, u.OnlyAPI)
	if perHour == 0 {
		// No limit configured for this user's tier is functionally the same as the
		// limiter being disabled from this caller's perspective.
		return rateLimitingDisabledResponder()
	}
	if slices.Contains(cfg.ElevatedUserIDs, u.Username()) {
		// Multiply the baseline limits by the configured multiplier for elevated users, if the multiplier is nonzero.
		if cfg.ElevatedUserMultiplier != 0 {
			perHour *= cfg.ElevatedUserMultiplier
			burst *= cfg.ElevatedUserMultiplier
		}
	}

	limiter, err := ratelimit.NewRateLimiter(h.env.RedisClient())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking rate limit status"))
	}
	if limiter == nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.New("nil rate limiter returned for rate limit status check"))
	}
	// Check the user's REST rate limit without consuming any tokens.
	result, err := limiter.Peek(ctx, u.Username(), evergreen.RateLimitSurfaceREST, perHour, burst)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking rate limit status"))
	}

	status := &restmodel.APIRateLimitStatus{}
	status.BuildFromService(result)
	return gimlet.NewJSONResponse(status)
}

// rateLimitingDisabledResponder is returned whenever there's no limit enforced
// against the caller, whether because the limiter is disabled globally or
// because their user type has no configured limit.
func rateLimitingDisabledResponder() gimlet.Responder {
	return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
		StatusCode: http.StatusConflict,
		Message:    "rate limiting is currently disabled",
	})
}
