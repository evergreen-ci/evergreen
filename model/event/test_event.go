package event

const ResourceTypeTest = "TEST"

type TestEvent struct {
	ResourceType string `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	Message      string `bson:"message" json:"message"`
}

func (e *TestEvent) IsValid() bool {
	return true
}
