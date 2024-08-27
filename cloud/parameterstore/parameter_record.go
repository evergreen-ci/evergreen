package parameterstore

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// ParameterRecord stores metadata information about parameters. This never
// stores the value of the parameter itself.
type ParameterRecord struct {
	// Name is the unique full path identifier for the parameter.
	Name string `bson:"_id" json:"_id"`
	// LastUpdated is the time the parameter was most recently updated.
	LastUpdated time.Time `bson:"last_updated" json:"last_updated"`
}

func (pr *ParameterRecord) Insert(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection(Collection).InsertOne(ctx, pr)
	return err
}
