package service

import (
	"net/http"
)

func (uis *UIServer) lobsterPage(w http.ResponseWriter, r *http.Request) {
	uis.render.WriteResponse(w, http.StatusOK, struct{}{}, "base", "lobster.html")
}
