package bond

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// ArtifactVersion represents a document in the Version field of the
// MongoDB build information feed. See
// http://downloads.mongodb.org/full.json for an example.ownload
type ArtifactVersion struct {
	Version   string
	Downloads []ArtifactDownload
	GitHash   string

	ProductionRelease  bool `json:"production_release"`
	DevelopmentRelease bool `json:"development_release"`
	Current            bool

	table map[BuildOptions]ArtifactDownload
	mutex sync.RWMutex
}

func (version *ArtifactVersion) refresh() {
	version.mutex.Lock()
	defer version.mutex.Unlock()

	version.table = make(map[BuildOptions]ArtifactDownload)

	for _, dl := range version.Downloads {
		version.table[dl.GetBuildOptions()] = dl
	}
}

// GetDownload returns a matching ArtifactDownload object
// given a BuildOptions object.
func (version *ArtifactVersion) GetDownload(key BuildOptions) (ArtifactDownload, error) {
	version.mutex.RLock()
	defer version.mutex.RLock()

	// TODO: this is the place to fix hanlding for the Base edition, which is not necessarily intuitive.
	if key.Edition == Base {
		if key.Target == "linux" {
			key.Target += "_" + string(key.Arch)
		}
	}

	// we look for debug builds later in the process, but as map
	// keys, debug is always false.
	key.Debug = false

	dl, ok := version.table[key]
	if !ok {
		return ArtifactDownload{}, errors.Errorf("there is no build for %s (%s) in edition %s",
			key.Target, key.Arch, key.Edition)
	}

	return dl, nil
}

// GetBuildTypes builds, from an ArtifactsVersion object a BuildTypes
// object that reports on the available builds for this version.
func (version *ArtifactVersion) GetBuildTypes() *BuildTypes {
	out := BuildTypes{}

	seenTargets := make(map[string]struct{})
	seenEditions := make(map[MongoDBEdition]struct{})
	seenArchitectures := make(map[MongoDBArch]struct{})

	for _, dl := range version.Downloads {
		out.Version = version.Version
		if dl.Edition == "source" {
			continue
		}

		if _, ok := seenTargets[dl.Target]; !ok {
			seenTargets[dl.Target] = struct{}{}
			out.Targets = append(out.Targets, dl.Target)
		}

		if _, ok := seenEditions[dl.Edition]; !ok {
			seenEditions[dl.Edition] = struct{}{}
			out.Editions = append(out.Editions, dl.Edition)
		}

		if _, ok := seenArchitectures[dl.Arch]; !ok {
			seenArchitectures[dl.Arch] = struct{}{}
			out.Architectures = append(out.Architectures, dl.Arch)
		}
	}

	return &out
}

func (version *ArtifactVersion) String() string {
	out := []string{version.Version}

	for _, dl := range version.Downloads {
		if dl.Edition == "source" {
			continue
		}

		out = append(out, fmt.Sprintf("\t target='%s', edition='%v', arch='%v'",
			dl.Target, dl.Edition, dl.Arch))
	}

	return strings.Join(out, "\n")
}
