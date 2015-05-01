package ui

import (
	"encoding/json"
	"fmt"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"sort"
)

func (uis *UIServer) distrosPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	distros, err := distro.Find(distro.All)

	if err != nil {
		message := fmt.Sprintf("error fetching distros: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	sort.Sort(&sortableDistro{distros})

	uis.WriteHTML(w, http.StatusOK, struct {
		Distros     []distro.Distro
		Keys        map[string]string
		User        *user.DBUser
		ProjectData projectContext
		Flashes     []interface{}
	}{distros, uis.Settings.Keys, GetUser(r), projCtx, PopFlashes(uis.CookieStore, r, w)},
		"base", "distros.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyDistro(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["distro_id"]

	u := MustHaveUser(r)

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		message := fmt.Sprintf("error reading request: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	oldDistro, err := distro.FindOne(distro.ById(id))
	if err != nil {
		message := fmt.Sprintf("error finding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	newDistro := *oldDistro
	newDistro.ProviderSettings = nil

	// attempt to unmarshal data into distros field for type validation
	if err = json.Unmarshal(b, &newDistro); err != nil {
		message := fmt.Sprintf("error unmarshaling request: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	// check that the resulting distro is valid
	vErrs := validator.CheckDistro(&newDistro, &uis.Settings, false)
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

	event.LogDistroModified(id, u.Username(), newDistro)

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Distro %v successfully updated.", id)))
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
	u := MustHaveUser(r)

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		message := fmt.Sprintf("error adding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var d distro.Distro

	if err = json.Unmarshal(b, &d); err != nil {
		message := fmt.Sprintf("error adding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	vErrs := validator.CheckDistro(&d, &uis.Settings, true)
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
