package event

func init() {
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectModified)
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectAdded)
}

const (
	EventResourceTypeProject = "PROJECT"
	EventTypeProjectModified = "PROJECT_MODIFIED"
	EventTypeProjectAdded    = "PROJECT_ADDED"
)
