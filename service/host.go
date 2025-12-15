package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (uis *UIServer) getHostDNS(r *http.Request) ([]string, error) {
	hostID := gimlet.GetVars(r)["host_id"]
	h, err := uis.getHostFromCache(r.Context(), hostID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get host '%s'", hostID)
	}
	if h == nil {
		return nil, errors.Wrapf(err, "host '%s' does not exist", hostID)
	}

	return []string{fmt.Sprintf("%s:%d", h.dnsName, evergreen.VSCodePort)}, nil
}

func (uis *UIServer) getHostFromCache(ctx context.Context, hostID string) (*hostCacheItem, error) {
	h, ok := uis.hostCache[hostID]
	if !ok || time.Since(h.inserted) > hostCacheTTL {
		hDb, err := host.FindOneId(ctx, hostID)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get host id '%s'", hostID)
		}
		if hDb == nil {
			return nil, nil
		}

		h = hostCacheItem{dnsName: hDb.Host, owner: hDb.StartedBy, isVirtualWorkstation: hDb.IsVirtualWorkstation, isRunning: hDb.Status == evergreen.HostRunning, inserted: time.Now()}
		uis.hostCache[hostID] = h
	}

	return &h, nil
}

func (uis *UIServer) handleBackendError(message string, statusCode int) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		gimlet.WriteTextResponse(w, statusCode, message)
	}
}

// returns dockerfle as text
func getDockerfile(w http.ResponseWriter, r *http.Request) {
	parts := []string{
		"ARG BASE_IMAGE",
		"FROM $BASE_IMAGE",
		"ARG URL",
		"ARG EXECUTABLE_SUB_PATH",
		"ARG BINARY_NAME",
		"ADD ${URL}/${EXECUTABLE_SUB_PATH} /",
		"RUN chmod 0777 /${BINARY_NAME}",
	}

	gimlet.WriteText(w, strings.Join(parts, "\n"))
}
