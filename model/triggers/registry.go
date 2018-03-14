package triggers

var triggerRegistry map[string][]trigger

func init() {
	triggerRegistry = map[string][]trigger{}
}

func getTriggers(resourceType string) []trigger {
	triggers, ok := triggerRegistry[resourceType]
	if !ok {
		return nil
	}

	return triggers
}

func hostTriggerFactory() []trigger {
	return nil
}
