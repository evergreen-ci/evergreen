package gimlet

import (
	"context"
	"net/http"
	"net/url"

	"github.com/mongodb/grip/message"
)

// UserMiddlewareConfiguration is an keyed-arguments struct used to
// produce the user manager middleware.
type UserMiddlewareConfiguration struct {
	SkipCookie      bool
	SkipHeaderCheck bool
	CookieName      string
	HeaderUserName  string
	HeaderKeyName   string
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

func (u *userMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	var err error
	ctx := r.Context()
	reqID := GetRequestID(ctx)
	logger := GetLogger(ctx)

	if !u.conf.SkipCookie {
		var token string

		// Grab token auth from cookies
		for _, cookie := range r.Cookies() {
			if cookie.Name == u.conf.CookieName {
				if token, err = url.QueryUnescape(cookie.Value); err == nil {
					break
				}
			}
		}

		// set the user, preferring the cookie, maye change
		if len(token) > 0 {
			ctx := r.Context()
			usr, err := u.manager.GetUserByToken(ctx, token)

			if err != nil {
				logger.Debug(message.WrapError(err, message.Fields{
					"request": reqID,
					"message": "problem getting user by token",
				}))
			} else {
				usr, err = u.manager.GetOrCreateUser(usr)
				// Get the user's full details from the DB or create them if they don't exists
				if err != nil {
					logger.Debug(message.WrapError(err, message.Fields{
						"message": "error looking up user",
						"request": reqID,
					}))
				}
			}

			if usr != nil {
				r = setUserForRequest(r, usr)
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

		if len(authDataAPIKey) > 0 {
			usr, err := u.manager.GetUserByID(authDataName)
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
