package ui

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
)

type RouteInfo struct {
	Path    string
	Handler http.HandlerFunc
	Name    string
	Method  string
}

type restUISAPI interface {
	WriteJSON(w http.ResponseWriter, status int, data interface{})
	GetSettings() evergreen.Settings
}

type restAPI struct {
	restUISAPI
}
