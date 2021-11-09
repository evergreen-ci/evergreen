package ecs

// Constants represents ECS task states.
const (
	// TaskStatusProvisioning indicates that ECS is performing additional work
	// before launching the task (e.g. provisioning a network interface for
	// AWSVPC).
	TaskStatusProvisioning = "PROVISIONING"
	// TaskStatusPending is a transition state indicating that ECS is waiting
	// for the container agent to act.
	TaskStatusPending = "PENDING"
	// TaskStatusActivating indicates that the task is launched but needs to
	// perform additional work before the task is fully running (e.g. service
	// discovery setup).
	TaskStatusActivating = "ACTIVATING"
	// TaskStatusRunning indicates that the task is running.
	TaskStatusRunning = "RUNNING"
	// TaskStatusDeactivating indicates that the task is preparing to stop but
	// needs to perform additional work first (e.g. deregistering load balancer
	// target groups).
	TaskStatusDeactivating = "DEACTIVATING"
	// TaskStatusStopping is a transition state indicating that ECS is waiting
	// for the container agent to act.
	TaskStatusStopping = "STOPPING"
	// TaskStatusDeprovisioning indicates that the task is no longer running but
	// needs to perform additional work before the task is fully stopped (e.g.
	// detaching the network interface for AWSVPC).
	TaskStatusDeprovisioning = "DEPROVISIONING"
	// TaskStatusStopped indicates that the task is stopped.
	TaskStatusStopped = "STOPPED"
)
