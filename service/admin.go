package service

import "net/http"

func (uis *UIServer) adminSettings(w http.ResponseWriter, r *http.Request) {
	DBUser := GetUser(r)
	template := "not_admin.html"
	if uis.isSuperUser(DBUser) {
		template = "admin.html"
	}
	uis.WriteHTML(w, http.StatusOK, uis.GetCommonViewData(w, r, true, true), "base", template, "base_angular.html", "menu.html")
}
