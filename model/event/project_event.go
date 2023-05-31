package event

func init() {
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectModified)
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectAdded)
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectAttachedToRepo)
	registry.setUnexpirable(EventResourceTypeProject, EventTypeProjectDetachedFromRepo)
}

const (
	EventResourceTypeProject         = "PROJECT"
	EventTypeProjectModified         = "PROJECT_MODIFIED"
	EventTypeProjectAdded            = "PROJECT_ADDED"
	EventTypeProjectAttachedToRepo   = "PROJECT_ATTACHED_TO_REPO"
	EventTypeProjectDetachedFromRepo = "PROJECT_DETACHED_FROM_REPO"
)
