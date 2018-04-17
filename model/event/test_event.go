package event

func init() {
	registry.AddType(ResourceTypeTest, testEventFactory)
	registry.AllowSubscription(ResourceTypeTest, "test")
}

const ResourceTypeTest = "TEST"

type TestEvent struct {
	Message string `bson:"message" json:"message"`
}
