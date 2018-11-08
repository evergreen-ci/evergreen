// Package db provides tools for using MongoDB databases. However, it
// wraps mgo types in interfaces that the anser/mocks package
// provides mocked implementations of for testing facility.
//
// In general, these types are fully functional for most application
// uses, but do not expose some of the configurability that the mgo
// equivalents do.
package db

// ChangeInfo represents the data returned by Update and Upsert
// documents. This type mirrors the mgo type.
type ChangeInfo struct {
	Updated    int         // Number of existing documents updated
	Removed    int         // Number of documents removed
	UpsertedId interface{} // Upserted _id field, when not explicitly provided
}
