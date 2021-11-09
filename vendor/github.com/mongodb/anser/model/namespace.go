package model

import "fmt"

// Namespace reflects a MongoDB database name and collection pair.
type Namespace struct {
	DB         string `bson:"db_name" json:"db_name" yaml:"db_name"`
	Collection string `bson:"collection" json:"collection" yaml:"collection"`
}

func (ns Namespace) String() string { return fmt.Sprintf("%s.%s", ns.DB, ns.Collection) }

// IsValid checks
func (ns Namespace) IsValid() bool {
	if ns.DB == "" {
		return false
	}

	if ns.Collection == "" {
		return false
	}

	if len(ns.DB) > 64 {
		return false
	}

	// this is not exhaustive, this doesn't check for restricted
	// characters or for very long collection names:
	//
	// - https://docs.mongodb.com/manual/reference/limits/#naming-restrictions
	// - https://docs.mongodb.com/manual/reference/limits/#namespaces

	return true
}
