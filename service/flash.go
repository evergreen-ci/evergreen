package service

import (
	"encoding/gob"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mongodb/grip"
)

const (
	FlashSeveritySuccess = "success"
	FlashSeverityInfo    = "info"
	FlashSeverityWarning = "warning"
	FlashSeverityError   = "danger"
)

const FlashSession = "mci-session"

type flashMessage struct {
	Severity string
	Message  string
}

func init() {
	gob.Register(&flashMessage{})
}

func NewSuccessFlash(message string) flashMessage {
	return flashMessage{Severity: FlashSeveritySuccess, Message: message}
}

func NewErrorFlash(message string) flashMessage {
	return flashMessage{Severity: FlashSeverityError, Message: message}
}

func PopFlashes(store *sessions.CookieStore, r *http.Request, w http.ResponseWriter) []interface{} {
	session, _ := store.Get(r, FlashSession)
	flashes := session.Flashes()
	grip.Warning(session.Save(r, w))
	return flashes
}

func PushFlash(store *sessions.CookieStore, r *http.Request, w http.ResponseWriter, msg flashMessage) {
	session, _ := store.Get(r, FlashSession)
	session.AddFlash(msg)
	grip.Warning(session.Save(r, w))
}
