package bond

// MongoDBEdition provides values that appear in the "edition" field
// of the feed, and map to specific builds of MongoDB.
type MongoDBEdition string

// Specific values for Editions.
const (
	Enterprise        MongoDBEdition = "enterprise"
	CommunityTargeted                = "targeted"
	Base                             = "base"
)

// MongoDBArch provides values that appear in the "arch" field of the
// feed and map onto specific processor architectures.
type MongoDBArch string

// Specific values for Architectures.
const (
	ZSeries MongoDBArch = "s390x"
	POWER               = "ppc64le"
	AMD64               = "x86_64"
	X86                 = "i686"
)
