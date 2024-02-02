package service

import (
	"fmt"
	"net/http"
)

func (uis *UIServer) distrosPage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/distros", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
	return
}
