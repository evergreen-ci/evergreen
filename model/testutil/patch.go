package testutil

import (
	"context"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

const PatchId = "58d156352cfeb61064cf08b3"

type PatchTestMode int

const (
	NoPatch PatchTestMode = iota
	InlinePatch
	ExternalPatch
	MergePatch
)

type PatchRequest struct {
	ModuleName string
	FilePath   string
	Githash    string
}

func SetupPatches(ctx context.Context, patchMode PatchTestMode, b *build.Build, patches ...PatchRequest) (*patch.Patch, error) {
	if patchMode == NoPatch {
		return nil, errors.New("no patch defined")
	}

	ptch := &patch.Patch{
		Id:      patch.NewId(PatchId),
		Status:  evergreen.VersionCreated,
		Version: b.Version,
		Patches: []patch.ModulePatch{},
	}

	for _, p := range patches {
		patchContent, err := os.ReadFile(p.FilePath)
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
			if err := db.WriteGridFile(ctx, patch.GridFSPrefix, ptch.Id.Hex(), strings.NewReader(string(patchContent))); err != nil {
				return nil, err
			}

			ptch.Patches = append(ptch.Patches, patch.ModulePatch{
				ModuleName: p.ModuleName,
				Githash:    p.Githash,
				PatchSet:   patch.PatchSet{PatchFileId: ptch.Id.Hex()},
			})
		}
	}
	return ptch, ptch.Insert(ctx)
}
