package event

const ResourceTypeTest = "TEST"

type TestEvent struct {
	Message string `bson:"message" json:"message"`
}
