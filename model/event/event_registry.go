package event

type eventFactory func() interface{}

var eventRegistry map[string]eventFactory

func init() {
	eventRegistry = map[string]eventFactory{
		ResourceTypeTask:      taskEventFactory,
		ResourceTypeHost:      hostEventFactory,
		ResourceTypeDistro:    distroEventFactory,
		ResourceTypeScheduler: schedulerEventFactory,
		EventTaskSystemInfo:   taskSystemResourceEventFactory,
		EventTaskProcessInfo:  taskProcessResourceEventFactory,
		ResourceTypeAdmin:     adminEventFactory,
	}
}

func NewEventFromType(resourceType string) interface{} {
	f, ok := eventRegistry[resourceType]
	if !ok {
		return nil
	}

	return f()
}

func taskEventFactory() interface{} {
	return &TaskEventData{}
}

func hostEventFactory() interface{} {
	return &HostEventData{}
}

func distroEventFactory() interface{} {
	return &DistroEventData{}
}

func schedulerEventFactory() interface{} {
	return &SchedulerEventData{}
}

func taskSystemResourceEventFactory() interface{} {
	return &TaskSystemResourceData{}
}

func taskProcessResourceEventFactory() interface{} {
	return &TaskProcessResourceData{}
}

func adminEventFactory() interface{} {
	return &rawAdminEventData{}
}

func isSubscribable(eventType string) bool {
	// TODO
	return false
}
