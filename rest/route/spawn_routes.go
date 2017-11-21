package route

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

func getSpawnHostsRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &spawnHostModifyHandler{},
				MethodType:        http.MethodGet,
			},
		},
		Version: version,
	}
}

// each regex matches one of the 5 categories listed here:
// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
var passwordRegexps = []*regexp.Regexp{
	regexp.MustCompile(`[\p{Ll}]`), // lowercase letter
	regexp.MustCompile(`[\p{Lu}]`), // uppercase letter
	regexp.MustCompile(`[0-9]`),
	regexp.MustCompile(`[~!@#$%^&*_\-+=|\\\(\){}\[\]:;"'<>,.?/` + "`]"),
	regexp.MustCompile(`[\p{Lo}]`), // letters without upper/lower variants (ex: Japanese)
}

const (
	HostPasswordUpdate         = "updateRDPPassword"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

type spawnHostModifyHandler struct {
	action      string
	hostID      string
	rdpPassword string
	addHours    time.Duration
}

func (h *spawnHostModifyHandler) Handler() RequestHandler {
	return &spawnHostModifyHandler{}
}

func (h *spawnHostModifyHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	h.action = string(hostModify.Action)
	if h.action != HostTerminate && h.action != HostExpirationExtension && h.action != HostPasswordUpdate {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid action",
		}
	}

	h.hostID = string(hostModify.HostID)
	if h.hostID == "" {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid host id",
		}
	}

	if h.action == HostPasswordUpdate {
		h.rdpPassword = string(hostModify.RDPPwd)

		if !validateRDPPassword(h.rdpPassword) {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid password",
			}
		}

	} else if h.action == HostExpirationExtension {
		addHours, err := strconv.Atoi(string(hostModify.AddHours))
		if err != nil {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "expiration not a number",
			}
		}
		extendBy := time.Duration(addHours) * time.Hour

		if extendBy <= time.Duration(0) {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "expiration must be greater than 0",
			}
		}
	}

	return nil
}

func (h *spawnHostModifyHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	host, err := sc.FindHostById(h.hostID)
	if err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if host == nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "unknown host id",
		}
	}

	if h.action == HostTerminate {
		if host.Status == evergreen.HostTerminated {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
			}

		} else if host.Status == evergreen.HostUninitialized {
			if err := sc.SetHostStatus(host, evergreen.HostTerminated); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}

		} else {
			if err := sc.TerminateHost(host); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}
		}

	} else if h.action == HostPasswordUpdate {
		if err := sc.SetHostPassword(ctx, host, h.rdpPassword); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}

	} else if h.action == HostExpirationExtension {
		newExp, err := makeNewHostExpiration(host, h.addHours)
		if err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}

		}

		if err := sc.SetHostExpirationTime(host, newExp); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
	}

	return ResponseData{}, nil
}

func makeNewHostExpiration(host *host.Host, addHours time.Duration) (time.Time, error) {
	newExp := host.ExpirationTime.Add(addHours)
	hoursUntilExpiration := time.Until(newExp).Hours()
	if hoursUntilExpiration > MaxExpirationDurationHours {
		return time.Time{}, errors.Errorf("Can not extend host '%s' expiration by '%d' hours. Maximum extension is limited to %d hours", host.Id, int(hoursUntilExpiration), MaxExpirationDurationHours)
	}

	return newExp, nil
}

func validateRDPPassword(password string) bool {
	// Golang regex doesn't support lookarounds, so we can't use
	// the regex as found in public/static/js/directives/directives.spawn.js
	if len([]rune(password)) < 6 || len([]rune(password)) > 255 {
		return false
	}

	// need to match 3 of 5 categories listed on:
	// https://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
	matchedCategories := 0
	for _, regex := range passwordRegexps {
		if regex.MatchString(password) {
			matchedCategories++
		}
	}

	return matchedCategories >= 3
}
