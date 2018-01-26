package service

import (
	"net/http"
)

func (uis *UIServer) perfdiscoveryPage(w http.ResponseWriter, r *http.Request) {
	uis.WriteHTML(w, http.StatusOK, struct {
		ViewData
	}{uis.GetCommonViewData(w, r, false, true)},
		"base", "perfdiscovery.html", "base_angular.html", "menu.html")
}
