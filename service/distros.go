package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func (uis *UIServer) distrosPage(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	permissions, err := rolemanager.HighestPermissionsForRolesAndResourceType(
		u.Roles(),
		evergreen.DistroResourceType,
		evergreen.GetEnvironment().RoleManager(),
	)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	distros, err := distro.Find(distro.All.Project(bson.M{"_id": 1}))
	if err != nil {
		message := fmt.Sprintf("error fetching distro ids: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	sort.Sort(&sortableDistro{distros})
	distroIds := []string{}
	for _, d := range distros {
		resourcePermissions, ok := permissions[d.Id]
		if ok && resourcePermissions[evergreen.PermissionDistroSettings] > 0 {
			distroIds = append(distroIds, d.Id)
		}
	}

	opts := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionDistroCreate,
		RequiredLevel: evergreen.DistroCreate.Value,
	}
	createDistro := u.HasPermission(opts)

	settings, err := evergreen.GetConfig()
	if err != nil {
		message := fmt.Sprintf("error fetching evergreen settings: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	containerPools := make([]evergreen.ContainerPool, 0)
	containerPoolDistros := make([]string, 0)
	containerPoolIds := make([]string, 0)
	for _, p := range settings.ContainerPools.Pools {
		containerPools = append(containerPools, p)
		containerPoolDistros = append(containerPoolDistros, p.Distro)
		containerPoolIds = append(containerPoolIds, p.Id)
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		CreateDistro     bool
		DistroIds        []string
		Keys             map[string]string
		ArchDisplayNames map[string]string
		ViewData
		ContainerPools                  []evergreen.ContainerPool
		ContainerPoolDistros            []string
		ContainerPoolIds                []string
		ValidHostAllocatorRoundingRules []string
		ValidHostAllocatorFeedbackRules []string
		ValidHostsOverallocatedRules    []string
	}{createDistro, distroIds, uis.Settings.Keys, evergreen.ValidArchDisplayNames,
		uis.GetCommonViewData(w, r, false, true), containerPools, containerPoolDistros, containerPoolIds, evergreen.ValidHostAllocatorRoundingRules, evergreen.ValidHostAllocatorFeedbackRules, evergreen.ValidHostsOverallocatedRules},
		"base", "distros.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyDistro(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["distro_id"]
	shouldDeco := r.FormValue("deco") == "true"
	shouldRestartJasper := r.FormValue("restart_jasper") == "true"
	shouldReprovisionToNew := r.FormValue("reprovision_to_new") == "true"

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

	newDistro := oldDistro
	newDistro.ProviderSettingsList = []*birch.Document{} // remove old list to prevent collisions within birch documents
	// attempt to unmarshal data into distros field for type validation
	if err = json.Unmarshal(b, &newDistro); err != nil {
		message := fmt.Sprintf("error unmarshaling request: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if newDistro.PlannerSettings.Version == "" {
		newDistro.PlannerSettings.Version = evergreen.PlannerVersionLegacy
	}
	if newDistro.BootstrapSettings.Method == "" {
		newDistro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
	}
	if newDistro.BootstrapSettings.Communication == "" {
		newDistro.BootstrapSettings.Communication = distro.BootstrapMethodLegacySSH
	}
	if newDistro.CloneMethod == "" {
		newDistro.CloneMethod = distro.CloneMethodLegacySSH
	}
	if newDistro.FinderSettings.Version == "" {
		newDistro.PlannerSettings.Version = evergreen.FinderVersionLegacy
	}

	settings, err := evergreen.GetConfig()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	// check that the resulting distro is valid
	vErrs, err := validator.CheckDistro(r.Context(), &newDistro, settings, false)
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
		gimlet.WriteJSONError(w, vErrs)
		return
	}

	if err = newDistro.Update(); err != nil {
		message := fmt.Sprintf("error updating distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if shouldDeco || shouldRestartJasper || shouldReprovisionToNew {
		hosts, err := host.Find(db.Query(host.ByDistroIDs(newDistro.Id)))
		if err != nil {
			message := fmt.Sprintf("error finding hosts: %s", err.Error())
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
			http.Error(w, message, http.StatusInternalServerError)
			return
		}

		if shouldDeco {
			err = host.DecommissionHostsWithDistroId(newDistro.Id)
			if err != nil {
				message := fmt.Sprintf("error decommissioning hosts: %s", err.Error())
				PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
				http.Error(w, message, http.StatusInternalServerError)
				return
			}
			for _, h := range hosts {
				event.LogHostStatusChanged(h.Id, h.Status, evergreen.HostDecommissioned, u.Username(), "distro page")
			}
		} else if shouldReprovisionToNew {
			catcher := grip.NewBasicCatcher()

			for _, h := range hosts {
				_, err = api.GetReprovisionToNewCallback(r.Context(), uis.env, u.Username())(&h)
				catcher.Wrapf(err, "marking host '%s' as needing to reprovision", h.Id)
			}

			if catcher.HasErrors() {
				message := fmt.Sprintf("error marking hosts as needing to reprovision: %s", err.Error())
				PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
				gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(errors.Wrap(err, "error marking hosts as needing to reprovision")))
				return
			}
		} else if shouldRestartJasper {
			catcher := grip.NewBasicCatcher()

			for _, h := range hosts {
				_, err = api.GetRestartJasperCallback(r.Context(), uis.env, u.Username())(&h)
				catcher.Wrapf(err, "marking host '%s' as needing Jasper service restarted", h.Id)
			}

			if catcher.HasErrors() {
				message := fmt.Sprintf("error marking hosts as needing Jasper service restarted: %s", err.Error())
				PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
				gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(errors.Wrap(err, "error marking hosts as needing Jasper service restarted")))
				return
			}
		}
	}

	if oldDistro.DispatcherSettings.Version == evergreen.DispatcherVersionRevisedWithDependencies && newDistro.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
		if err := model.RemoveTaskQueues(id); err != nil {
			PushFlash(uis.CookieStore, r, w, NewWarningFlash(err.Error()))
		}
	}

	event.LogDistroModified(id, u.Username(), newDistro.NewDistroData())

	message := fmt.Sprintf("Distro %v successfully updated.", id)
	if shouldDeco {
		message = fmt.Sprintf("Distro %v successfully updated and running hosts decommissioned", id)
	}
	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(message))
	gimlet.WriteJSON(w, "distro successfully updated")
}

func (uis *UIServer) removeDistro(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["distro_id"]

	u := MustHaveUser(r)

	d, err := distro.FindByID(id)
	if err != nil {
		message := fmt.Sprintf("error finding distro: %v", err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}
	if d == nil {
		message := fmt.Sprintf("distro '%s' doesn't exist", id)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	if err = host.MarkInactiveStaticHosts([]string{}, d); err != nil {
		message := fmt.Sprintf("error removing hosts for distro '%s': %s", id, err)
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
	if err = model.ClearTaskQueue(id); err != nil {
		message := fmt.Sprintf("clearing task queue for distro '%s': %s", id, err.Error())
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	event.LogDistroRemoved(id, u.Username(), d.NewDistroData())

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Distro %v successfully removed.", id)))
	gimlet.WriteJSON(w, "distro successfully removed")
}

func (uis *UIServer) getDistro(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["distro_id"]

	u := MustHaveUser(r)

	d, err := distro.FindOne(distro.ById(id))
	if err != nil {
		message := fmt.Sprintf("error fetching distro '%v': %v", id, err)
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(message))
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	regions := uis.Settings.Providers.AWS.AllowedRegions
	opts := gimlet.PermissionOpts{Resource: id, ResourceType: evergreen.DistroResourceType}
	permissions, err := rolemanager.HighestPermissionsForRoles(u.Roles(), evergreen.GetEnvironment().RoleManager(), opts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	data := struct {
		Distro      distro.Distro      `json:"distro"`
		Regions     []string           `json:"regions"`
		Permissions gimlet.Permissions `json:"permissions"`
	}{d, regions, permissions}

	gimlet.WriteJSON(w, data)
}

func (uis *UIServer) addDistro(w http.ResponseWriter, r *http.Request) {
	id, hasId := gimlet.GetVars(r)["distro_id"]

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
	settings, err := evergreen.GetConfig()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	vErrs, err := validator.CheckDistro(r.Context(), &d, settings, true)
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
		gimlet.WriteJSONError(w, vErrs)
		return
	}

	if err = d.Add(u); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error adding distro",
		}))
		errMsg := fmt.Sprintf("error adding distro")
		PushFlash(uis.CookieStore, r, w, NewErrorFlash(errMsg))
		gimlet.WriteJSONInternalError(w, err)
		return
	}

	event.LogDistroAdded(d.Id, u.Username(), d.NewDistroData())

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Distro %v successfully added.", d.Id)))
	gimlet.WriteJSON(w, "distro successfully added")
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
