package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/gorilla/mux"
)

func (uis *UIServer) distrosPage(w http.ResponseWriter, r *http.Request) {
	distros, err := distro.Find(distro.All)

	if err != nil {
		message := fmt.Sprintf("error fetching distros: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	sort.Sort(&sortableDistro{distros})

	uis.WriteHTML(w, http.StatusOK, struct {
		Distros []distro.Distro
		Keys    map[string]string
		ViewData
	}{distros, uis.Settings.Keys, uis.GetCommonViewData(w, r, false, true)},
		"base", "distros.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyDistro(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["distro_id"]
	shouldDeco := r.FormValue("deco") == "true"

	u := MustHaveUser(r)

	body := util.NewRequestReader(r)
	defer body.Close()

	b, err := ioutil.ReadAll(body)
	if err != nil {
		message := fmt.Sprintf("error reading request: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	oldDistro, err := distro.FindOne(distro.ById(id))
	if err != nil {
		message := fmt.Sprintf("error finding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	newDistro := *oldDistro

	// attempt to unmarshal data into distros field for type validation
	if err = json.Unmarshal(b, &newDistro); err != nil {
		message := fmt.Sprintf("error unmarshaling request: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	// check that the resulting distro is valid
	vErrs, err := validator.CheckDistro(&newDistro, &uis.Settings, false)
	if err != nil {
		message := fmt.Sprintf("error retrieving distroIds: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	if len(vErrs) != 0 {
		for _, e := range vErrs {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(e.Error()))
		}
		uis.WriteJSON(w, http.StatusBadRequest, vErrs)
		return
	}

	if err = newDistro.Update(); err != nil {
		message := fmt.Sprintf("error updating distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if shouldDeco {
		err = host.DecommissionHostsWithDistroId(newDistro.Id)
		if err != nil {
			message := fmt.Sprintf("error decommissioning hosts: %v", err)
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
			http.Error(w, message, http.StatusBadRequest)
			return
		}
	}

	event.LogDistroModified(id, u.Username(), newDistro)

	message := fmt.Sprintf("Distro %v successfully updated.", id)
	if shouldDeco {
		message = fmt.Sprintf("Distro %v successfully updated and running hosts decommissioned", id)
	}
	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(message))
	uis.WriteJSON(w, http.StatusOK, "distro successfully updated")
}

func (uis *UIServer) removeDistro(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["distro_id"]

	u := MustHaveUser(r)

	d, err := distro.FindOne(distro.ById(id))
	if err != nil {
		message := fmt.Sprintf("error finding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	if err = distro.Remove(id); err != nil {
		message := fmt.Sprintf("error removing distro '%v': %v", id, err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	event.LogDistroRemoved(id, u.Username(), d)

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Distro %v successfully removed.", id)))
	uis.WriteJSON(w, http.StatusOK, "distro successfully removed")
}

func (uis *UIServer) getDistro(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["distro_id"]

	d, err := distro.FindOne(distro.ById(id))
	if err != nil {
		message := fmt.Sprintf("error fetching distro '%v': %v", id, err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	uis.WriteJSON(w, http.StatusOK, d)
}

func (uis *UIServer) addDistro(w http.ResponseWriter, r *http.Request) {
	id, hasId := mux.Vars(r)["distro_id"]

	u := MustHaveUser(r)

	body := util.NewRequestReader(r)
	defer body.Close()

	b, err := ioutil.ReadAll(body)
	if err != nil {
		message := fmt.Sprintf("error adding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var d distro.Distro

	if err = json.Unmarshal(b, &d); err != nil {
		message := fmt.Sprintf("error adding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if hasId {
		d.Id = id
	}

	vErrs, err := validator.CheckDistro(&d, &uis.Settings, true)
	if err != nil {
		message := fmt.Sprintf("error retrieving distroIds: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	if len(vErrs) != 0 {
		for _, e := range vErrs {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(e.Error()))
		}
		uis.WriteJSON(w, http.StatusBadRequest, vErrs)
		return
	}

	if err = d.Insert(); err != nil {
		message := fmt.Sprintf("error inserting distro '%v': %v", d.Id, err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		uis.WriteJSON(w, http.StatusInternalServerError, err)
		return
	}

	event.LogDistroAdded(d.Id, u.Username(), d)

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Distro %v successfully added.", d.Id)))
	uis.WriteJSON(w, http.StatusOK, "distro successfully added")
}

type sortableDistro struct {
	distros []distro.Distro
}

func (s *sortableDistro) Len() int {
	return len(s.distros)
}

func (s *sortableDistro) Less(i, j int) bool {
	return s.distros[i].Id < s.distros[j].Id
}

func (s *sortableDistro) Swap(i, j int) {
	s.distros[i], s.distros[j] = s.distros[j], s.distros[i]
}
