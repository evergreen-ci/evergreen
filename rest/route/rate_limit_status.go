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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
//	@Description	Get the caller's current REST rate limit status. Callers may only check their own rate limit. Returns null if the caller has no limit configured or the rate limiter is disabled.
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
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "getting service flags for rate limit status check",
			"user":    u.Username(),
		}))
		return gimlet.NewJSONResponse(nil)
	}
	if flags.APIRateLimiterDisabled {
		return gimlet.NewJSONResponse(nil)
	}

	cfg := h.env.Settings().RateLimit
	perHour, burst := limitsFor(&cfg, evergreen.RateLimitSurfaceREST, u.OnlyAPI)
	if perHour == 0 {
		return gimlet.NewJSONResponse(nil)
	}
	if slices.Contains(cfg.ElevatedUserIDs, u.Username()) {
		perHour *= 2
		burst *= 2
	}

	limiter, err := ratelimit.NewRateLimiter(h.env.RedisClient())
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "initializing rate limiter for rate limit status check",
			"user":    u.Username(),
		}))
		return gimlet.NewJSONResponse(nil)
	}
	if limiter == nil {
		grip.Warning(ctx, "nil rate limiter returned for rate limit status check")
		return gimlet.NewJSONResponse(nil)
	}
	// Check the user's REST rate limit without consuming any tokens.
	result, err := limiter.Peek(ctx, u.Username(), evergreen.RateLimitSurfaceREST, perHour, burst)
	if err != nil {
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message": "checking rate limit status",
			"user":    u.Username(),
		}))
		return gimlet.NewJSONResponse(nil)
	}

	status := &restmodel.APIRateLimitStatus{}
	status.BuildFromService(result)
	return gimlet.NewJSONResponse(status)
}
