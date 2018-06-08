package service

import (
	"net/http"
)

func (uis *UIServer) notificationsPage(w http.ResponseWriter, r *http.Request) {
	currentUser := GetUser(r)
	if currentUser == nil {
		http.Error(w, "No user logged in", http.StatusUnauthorized)
		return
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		ViewData
	}{uis.GetCommonViewData(w, r, true, true)},
		"base", "notifications.html", "base_angular.html", "menu.html")
}
