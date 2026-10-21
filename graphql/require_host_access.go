package graphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
)

func RequireHostAccess(ctx context.Context, obj any, next graphql.Resolver, access HostAccessLevel) (any, error) {
	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, ResourceNotFound.Send(ctx, "converting args into map")
	}
	hostId, hasHostId := args["hostId"].(string)
	hostIdsInterface, hasHostIds := args["hostIds"].([]any)

	// If no host ID is present, the field is optional and null, so skip the access check.
	if !hasHostId && !hasHostIds {
		return next(ctx)
	}
	if (hasHostId && hostId == "") || (hasHostIds && len(hostIdsInterface) == 0) {
		return nil, ResourceNotFound.Send(ctx, "must specify host ID(s)")
	}

	hostIdsToCheck := []string{hostId}
	if hasHostIds {
		for _, v := range hostIdsInterface {
			hostIdsToCheck = append(hostIdsToCheck, v.(string))
		}
	}
	var requiredLevel int
	if access == HostAccessLevelEdit {
		requiredLevel = evergreen.HostsEdit.Value
	} else {
		requiredLevel = evergreen.HostsView.Value
	}
	user := mustHaveUser(ctx)
	hostsToCheck, err := host.Find(ctx, host.ByIds(hostIdsToCheck))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting hosts: %s", err.Error()))
	}
	if len(hostsToCheck) == 0 {
		return nil, ResourceNotFound.Send(ctx, "No matching hosts found")
	}
	forbiddenHosts := []string{}
	for _, h := range hostsToCheck {
		if !userHasHostPermission(ctx, user, h.Distro.Id, requiredLevel, h.StartedBy) {
			forbiddenHosts = append(forbiddenHosts, h.Id)
		}
	}
	if len(forbiddenHosts) == 1 {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access host '%s'", user.Username(), forbiddenHosts[0]))
	} else if len(forbiddenHosts) > 1 {
		hostsString := strings.Join(forbiddenHosts, ", ")
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access hosts: '%s'", user.Username(), hostsString))
	}

	return next(ctx)
}
