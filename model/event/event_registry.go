package event

type eventFactory func() Data

var eventRegistry map[string]eventFactory

func init() {
	eventRegistry = map[string]eventFactory{
		ResourceTypeTask:         taskEventFactory,
		ResourceTypeHost:         hostEventFactory,
		ResourceTypeDistro:       distroEventFactory,
		ResourceTypeScheduler:    schedulerEventFactory,
		EventTaskSystemInfo:      taskSystemResourceEventFactory,
		EventTaskProcessInfo:     taskProcessResourceEventFactory,
		ResourceTypeAdmin:        adminEventFactory,
		ResourceTypeNotification: notificationEventFactory,
	}
}

func NewEventFromType(resourceType string) Data {
	f, ok := eventRegistry[resourceType]
	if !ok {
		return nil
	}

	return f()
}

func taskEventFactory() Data {
	return &TaskEventData{
		ResourceType: ResourceTypeTask,
	}
}

func hostEventFactory() Data {
	return &HostEventData{
		ResourceType: ResourceTypeHost,
	}
}

func distroEventFactory() Data {
	return &DistroEventData{
		ResourceType: ResourceTypeDistro,
	}
}

func schedulerEventFactory() Data {
	return &SchedulerEventData{
		ResourceType: ResourceTypeScheduler,
	}
}

func taskSystemResourceEventFactory() Data {
	return &TaskSystemResourceData{
		ResourceType: EventTaskSystemInfo,
	}
}

func taskProcessResourceEventFactory() Data {
	return &TaskProcessResourceData{
		ResourceType: EventTaskProcessInfo,
	}
}

func adminEventFactory() Data {
	return &rawAdminEventData{
		ResourceType: ResourceTypeAdmin,
	}
}

func notificationEventFactory() Data {
	return &NotificationEvent{
		ResourceType: ResourceTypeNotification,
	}
}
