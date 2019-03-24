package bond

// ArtifactDownload represents a single build or download in the
// MongoDB build artifacts feed. A version has many builds. See
// http://downloads.mongodb.org/full.json for an example.
type ArtifactDownload struct {
	Arch    MongoDBArch
	Edition MongoDBEdition
	Target  string
	Archive struct {
		Debug  string `bson:"debug_symbols" json:"debug_symbols" yaml:"debug_symbols"`
		Sha1   string
		Sha256 string
		URL    string `bson:"url" json:"url" yaml:"url"`
	}
	Msi      string
	Packages []string
}

// GetBuildOptions returns a BuildOptions object that represents the
// download.
func (dl ArtifactDownload) GetBuildOptions() BuildOptions {
	opts := BuildOptions{
		Target:  dl.Target,
		Arch:    dl.Arch,
		Edition: dl.Edition,
	}

	return opts
}

// GetPackages returns a slice of urls of all packages for a given build.
func (dl ArtifactDownload) GetPackages() []string {
	if dl.Msi != "" && len(dl.Packages) == 0 {
		return []string{dl.Msi}
	}

	return dl.Packages
}

// GetArchive returns the URL for the artifacts archive for a given
// download or build.
func (dl ArtifactDownload) GetArchive() string {
	return dl.Archive.URL
}
