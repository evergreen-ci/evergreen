package bond

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

// BuildOptions is a common method to describe a build variant.
type BuildOptions struct {
	Target  string         `json:"target"`
	Arch    MongoDBArch    `json:"arch"`
	Edition MongoDBEdition `json:"edition"`
	Debug   bool           `json:"debug"`
}

func (o BuildOptions) String() string {
	out, err := json.Marshal(o)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// GetBuildInfo given a version string, generates a BuildInfo object
// from a BuildOptions object.
func (o BuildOptions) GetBuildInfo(version string) BuildInfo {
	return BuildInfo{
		Version: version,
		Options: o,
	}
}

// Validate checks a BuildOption structure and ensures that there are
// no errors.
func (o BuildOptions) Validate() error {
	var errs []string

	if o.Target == "" {
		errs = append(errs, "target definition is missing")
	}

	if o.Arch == "" {
		errs = append(errs, "arch definition is missing")
	}

	if o.Edition == "" {
		errs = append(errs, "edition definition is missing")
	}

	if len(errs) != 0 {
		return errors.Errorf("%d errors: [%s]",
			len(errs), strings.Join(errs, "; "))
	}

	return nil
}
