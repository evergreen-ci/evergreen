package event

func init() {
	registryAdd(ResourceTypeTest, testEventFactory)
	allowSubscriptions(ResourceTypeTest, "test")
}

const ResourceTypeTest = "TEST"

type TestEvent struct {
	Message string `bson:"message" json:"message"`
}
