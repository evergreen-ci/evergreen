package global

// Global stores internal global tracking information for each build variant.
type Global struct {
	// BuildVariant is the name of the stored build variant.
	BuildVariant string `bson:"_id"`
	// LastBuildNumber is the counter for the build number for a particular
	// build variant.
	LastBuildNumber uint64 `bson:"last_build_number"`
	// LastTaskNumber is a deprecated field.
	LastTaskNumber uint64 `bson:"last_task_number"`
}
