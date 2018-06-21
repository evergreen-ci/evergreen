package gimlet

import (
	"context"
	"net/http"
	"net/url"

	"github.com/mongodb/grip/message"
)

type UserMiddlewareConfiguration struct {
	SkipCookie      bool
	SkipHeaderCheck bool
	CookieName      string
	HeaderUserName  string
	HeaderKeyName   string
}

func setUserForRequest(r *http.Request, u User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, u))
}

func GetUser(ctx context.Context) (User, bool) {
	u := ctx.Value(userKey)
	if u == nil {
		return nil, false
	}

	usr, ok := u.(User)
	if !ok {
		return nil, false
	}

	return usr, true
}

type userMiddleware struct {
	conf    UserMiddlewareConfiguration
	manager UserManager
}

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
				} else {
					r = setUserForRequest(r, usr)
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

		if len(authDataAPIKey) > 0 {
			usr, err := u.manager.GetUserByID(authDataName)

			// only loggable if the err is non-nil
			logger.Error(message.WrapError(err, message.Fields{
				"message":   "problem getting user by id",
				"operation": "header check",
				"name":      authDataName,
				"request":   reqID,
			}))

			if usr != nil {
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
