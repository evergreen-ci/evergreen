package model

// APIRepo identifies a repo resource for access-management integrations.
type APIRepo struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Admins []string `json:"admins"`
}
