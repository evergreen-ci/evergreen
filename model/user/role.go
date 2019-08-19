package user

const (
	RoleCollection = "roles"
)

type Role struct {
	Id          string            `bson:"_id"`
	Name        string            `bson:"name"`
	ScopeType   ScopeType         `bson:"scope_type"`
	Scope       string            `bson:"scope"`
	Permissions map[string]string `bson:"permissions"`
}

type ScopeType string

const (
	ScopeTypeProject     ScopeType = "project"
	ScopeTypeAllProjects           = "project_all"
	ScopeTypeDistro                = "distro"
	ScopeTypeAllDistros            = "distro_all"
)
