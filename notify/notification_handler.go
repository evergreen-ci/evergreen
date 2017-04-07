package notify

import (
	"github.com/evergreen-ci/evergreen/web"
)

// General interface for how notify.go deals with a given type of notification.
// At a high level, structs that implement this interface are responsible for
// scanning the database to find which notifications to generate and returning
// an object matching the Email interface for each notification.
type NotificationHandler interface {
	// Given a key, scan through database and generate the notifications that
	// have been triggered since the last time this function ran. Returns the
	// emails generated for this particular key.
	GetNotifications(ae *web.App, key *NotificationKey) ([]Email, error)
}
