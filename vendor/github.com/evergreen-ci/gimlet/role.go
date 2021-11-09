package gimlet

// Role is the data structure used to read and manipulate user roles and permissions
type Role struct {
	ID          string      `json:"id" bson:"_id"`
	Name        string      `json:"name" bson:"name"`
	Scope       string      `json:"scope" bson:"scope"`
	Permissions Permissions `json:"permissions" bson:"permissions"`
	Owners      []string    `json:"owners" bson:"owners"`
}

// Scope describes one or more resources and can be used to roll up certain resources into others
type Scope struct {
	ID          string   `json:"id" bson:"_id"`
	Name        string   `json:"name" bson:"name"`
	Type        string   `json:"type" bson:"type"`
	Resources   []string `json:"resources" bson:"resources"`
	ParentScope string   `json:"parent" bson:"parent"`
}
