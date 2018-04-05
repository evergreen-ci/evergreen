package model

import "errors"

type APIVersions struct {
	// whether or not the version element actually consists of multiple inactive
	// versions rolled up into one
	RolledUp bool `json:"rolled_up"`

	Versions []APIVersion `json:"versions"`
}

type BuildList struct {
	BuildVariant string              `json:"build_variant"`
	Builds       map[string]APIBuild `json:"builds"`
}

type VersionVariantData struct {
	Rows          map[string]BuildList `json:"rows"`
	Versions      []APIVersions        `json:"versions"`
	BuildVariants []string             `json:"build_variants"`
}

func (v *VersionVariantData) BuildFromService(h interface{}) error {
	return nil
}

func (v *VersionVariantData) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for VersionVariantData")
}
