package testutil

import (
	"io/ioutil"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

// the revision we'll assume is the current one for the agent. this is the
// same as appears in testdata/executables/version
var agentRevision = "xxx"

const PatchId = "58d156352cfeb61064cf08b3"

type PatchTestMode int

const (
	NoPatch PatchTestMode = iota
	InlinePatch
	ExternalPatch
)

func (m PatchTestMode) String() string {
	switch m {
	case NoPatch:
		return "none"
	case InlinePatch:
		return "inline"
	case ExternalPatch:
		return "external"
	}

	return "unknown"
}

type PatchRequest struct {
	ModuleName string
	FilePath   string
	Githash    string
}

func SetupPatches(patchMode PatchTestMode, b *build.Build, patches ...PatchRequest) (*patch.Patch, error) {
	if patchMode == NoPatch {
		return nil, errors.New("no patch defined")
	}

	ptch := &patch.Patch{
		Id:      patch.NewId(PatchId),
		Status:  evergreen.PatchCreated,
		Version: b.Version,
		Patches: []patch.ModulePatch{},
	}

	for _, p := range patches {
		patchContent, err := ioutil.ReadFile(p.FilePath)
		if err != nil {
			return nil, err
		}

		if patchMode == InlinePatch {
			ptch.Patches = append(ptch.Patches, patch.ModulePatch{
				ModuleName: p.ModuleName,
				Githash:    p.Githash,
				PatchSet:   patch.PatchSet{Patch: string(patchContent)},
			})
		} else {
			if err := db.WriteGridFile(patch.GridFSPrefix, string(ptch.Id), strings.NewReader(string(patchContent))); err != nil {
				return nil, err
			}

			ptch.Patches = append(ptch.Patches, patch.ModulePatch{
				ModuleName: p.ModuleName,
				Githash:    p.Githash,
				PatchSet:   patch.PatchSet{PatchFileId: string(ptch.Id)},
			})
		}
	}
	return ptch, ptch.Insert()
}
