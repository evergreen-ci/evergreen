package gimlet

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// UserMiddlewareConfiguration is an keyed-arguments struct used to
// produce the user manager middleware.
type UserMiddlewareConfiguration struct {
	SkipCookie      bool
	SkipHeaderCheck bool
	HeaderUserName  string
	HeaderKeyName   string
	CookieName      string
	CookiePath      string
	CookieTTL       time.Duration
	CookieDomain    string
}

// Validate ensures that the UserMiddlewareConfiguration is correct
// and internally consistent.
func (umc *UserMiddlewareConfiguration) Validate() error {
	catcher := grip.NewBasicCatcher()

	if !umc.SkipCookie {
		if umc.CookieName == "" {
			catcher.New("must specify cookie name when cookie authentication is enabled")
		}

		if umc.CookieTTL < time.Second {
			catcher.New("cookie timeout is less than a second")
		}

		if umc.CookiePath == "" {
			umc.CookiePath = "/"
		} else if !strings.HasPrefix(umc.CookiePath, "/") {
			catcher.New("cookie path must begin with '/'")
		}
	}

	if !umc.SkipHeaderCheck {
		if umc.HeaderUserName == "" {
			catcher.New("when header auth is enabled, must specify a header user name")
		}

		if umc.HeaderKeyName == "" {
			catcher.New("when header auth is enabled, must specify a header key name")
		}
	}

	return catcher.Resolve()
}

// AttachCookie sets a cookie with the specified cookie to the
// request, according to the configuration of the user manager.
func (umc UserMiddlewareConfiguration) AttachCookie(token string, rw http.ResponseWriter) {
	http.SetCookie(rw, &http.Cookie{
		Name:     umc.CookieName,
		Path:     umc.CookiePath,
		Value:    token,
		HttpOnly: true,
		Expires:  time.Now().Add(umc.CookieTTL),
		Domain:   umc.CookieDomain,
	})
}

// ClearCookie removes the cookie defied in the user manager.
func (umc UserMiddlewareConfiguration) ClearCookie(rw http.ResponseWriter) {
	http.SetCookie(rw, &http.Cookie{
		Name:   umc.CookieName,
		Path:   umc.CookiePath,
		Domain: umc.CookieDomain,
		Value:  "",
		MaxAge: -1,
	})
}

func setUserForRequest(r *http.Request, u User) *http.Request {
	return r.WithContext(AttachUser(r.Context(), u))
}

// AttachUser adds a user to a context. This function is public to
// support teasing workflows.
func AttachUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// GetUser returns the user attached to the request. The User object
// is nil when
func GetUser(ctx context.Context) User {
	u := ctx.Value(userKey)
	if u == nil {
		return nil
	}

	usr, ok := u.(User)
	if !ok {
		return nil
	}

	return usr
}

type userMiddleware struct {
	conf    UserMiddlewareConfiguration
	manager UserManager
}

// UserMiddleware produces a middleware that parses requests and uses
// the UserManager attached to the request to find and attach a user
// to the request.
func UserMiddleware(um UserManager, conf UserMiddlewareConfiguration) Middleware {
	return &userMiddleware{
		conf:    conf,
		manager: um,
	}
}

var ErrNeedsReauthentication = errors.New("user session has expired so they must be reauthenticated")

func (u *userMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) { //nolint: gocyclo
	var err error
	var usr User
	ctx := r.Context()
	reqID := GetRequestID(ctx)
	logger := GetLogger(ctx)

	if !u.conf.SkipCookie {
		var token string

		// Grab token auth from cookies
		for _, cookie := range r.Cookies() {
			if cookie.Name == u.conf.CookieName {
				if token, err = url.QueryUnescape(cookie.Value); err == nil {
					// set the user, preferring the cookie, maybe change
					if len(token) > 0 {
						usr, err = u.manager.GetUserByToken(ctx, token)
						needsReauth := errors.Cause(err) == ErrNeedsReauthentication

						logger.DebugWhen(err != nil && !needsReauth, message.WrapError(err, message.Fields{
							"request": reqID,
							"message": "problem getting user by token",
						}))
						if err == nil {
							usr, err = u.manager.GetOrCreateUser(usr)
							// Get the user's full details from the DB or create them if they don't exists
							if err != nil {
								logger.Debug(message.WrapError(err, message.Fields{
									"message": "error looking up user",
									"request": reqID,
								}))
							}
						}

						if usr != nil && !needsReauth {
							r = setUserForRequest(r, usr)
							break
						}
					}
				}
			}
		}
	}

	if !u.conf.SkipHeaderCheck {
		var (
			authDataAPIKey string
			authDataName   string
		)

		// Grab API auth details from header
		if len(r.Header[u.conf.HeaderKeyName]) > 0 {
			authDataAPIKey = r.Header[u.conf.HeaderKeyName][0]
		}
		if len(r.Header[u.conf.HeaderUserName]) > 0 {
			authDataName = r.Header[u.conf.HeaderUserName][0]
		}

		if len(authDataName) > 0 && len(authDataAPIKey) > 0 {
			usr, err = u.manager.GetUserByID(authDataName)
			logger.Debug(message.WrapError(err, message.Fields{
				"message":   "problem getting user by id",
				"operation": "header check",
				"name":      authDataName,
				"request":   reqID,
			}))

			// only loggable if the err is non-nil
			if err == nil && usr != nil {
				if usr.GetAPIKey() != authDataAPIKey {
					WriteTextResponse(rw, http.StatusUnauthorized, "invalid API key")
					return
				}
				r = setUserForRequest(r, usr)
			}
		}
	}

	next(rw, r)
}
