package event

func init() {
	registry.setNeverExpire(EventResourceTypeProject, EventTypeProjectModified)
	registry.setNeverExpire(EventResourceTypeProject, EventTypeProjectAdded)
}

const (
	EventResourceTypeProject = "PROJECT"
	EventTypeProjectModified = "PROJECT_MODIFIED"
	EventTypeProjectAdded    = "PROJECT_ADDED"
)
