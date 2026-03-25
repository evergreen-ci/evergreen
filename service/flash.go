package service

import (
	"encoding/gob"
	"net/http"

	"context"
	"github.com/gorilla/sessions"
	"github.com/mongodb/grip"
)

const (
	FlashSeveritySuccess = "success"

	FlashSeverityError = "danger"
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

func PopFlashes(store *sessions.CookieStore, r *http.Request, w http.ResponseWriter) []any {
	ctx := context.TODO()
	session, _ := store.Get(r, FlashSession)
	flashes := session.Flashes()
	grip.Warning(ctx, session.Save(r, w))
	return flashes
}

func PushFlash(store *sessions.CookieStore, r *http.Request, w http.ResponseWriter, msg flashMessage) {
	ctx := context.TODO()
	session, _ := store.Get(r, FlashSession)
	session.AddFlash(msg)
	grip.Warning(ctx, session.Save(r, w))
}
