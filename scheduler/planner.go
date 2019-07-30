package scheduler

import (
	"crypto/sha1"
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
)

// UnitCache stores an unordered collection of schedulable units. The
// Unit type holds one or more tasks, but is handled by the scheduler
// as a single object. While the constituent tasks in a unit have an
// order, the unit themselves are an intermediate abstraction for the
// planner which represent task groups, tasks with their dependencies,
// or the tasks from a single version
type UnitCache map[string]*Unit

// AddWhen wraps AddNew, and is a noop if the conditional is false.
func (cache UnitCache) AddWhen(cond bool, id string, unit *Unit) {
	if !cond {
		return
	}

	cache.AddNew(id, unit)
}

// AddNew adds an entire unit to a cache with the specified ID. If the
// cached item exists, AddNew extends the existing unit with the tasks
// from the passed unit.
func (cache UnitCache) AddNew(id string, unit *Unit) {
	if existing, ok := cache[id]; ok {
		for _, t := range unit.tasks {
			existing.Add(t)
		}

		return
	}

	cache[id] = unit
}

// Create makes a new unit around the existing task, caching it with
// the specified key, and returning the resulting unit. If there is an
// existing cache item with the specified ID, then Create extends that
// unit with this task. In both cases, the resulting unit is returned
// to the caller.
func (cache UnitCache) Create(id string, t task.Task) *Unit {
	if unit, ok := cache[id]; ok {
		unit.Add(t)
		return unit
	}

	unit := NewUnit(t)
	cache.AddNew(id, unit)
	return unit
}

// Export returns an unordered sequence of unique Units.
func (cache UnitCache) Export() TaskPlan {
	seen := StringSet{}
	tpl := TaskPlan{}
	for id := range cache {
		if seen.Visit(cache[id].ID()) {
			continue
		}

		tpl = append(tpl, cache[id])
	}

	return tpl
}

// Unit is a holder of a group of related tasks which should be
// scheculded together. Typically these represent task groups, tasks,
// and their dependencies, or even all tasks of a version. All tasks
// in a Unit must be unique with regards to their ID.
type Unit struct {
	tasks       map[string]task.Task
	cachedValue int64
	id          string
	distro      *distro.Distro
}

// NewUnit constructs a new Unit container for a task.
func NewUnit(t task.Task) *Unit { return &Unit{tasks: map[string]task.Task{t.Id: t}} }

// Export returns an unordered sequence of tasks from unit. All tasks
// are unique.
func (unit *Unit) Export() TaskList {
	out := make(TaskList, 0, len(unit.tasks))

	for _, t := range unit.tasks {
		out = append(out, t)
	}

	return out
}

// Add caches a task in the unit.
func (unit *Unit) Add(t task.Task) { unit.tasks[t.Id] = t }

// ID constructs a unique and hashed ID of all the tasks in the unit.
func (unit *Unit) ID() string {
	if unit.id != "" {
		return unit.id
	}

	hash := sha1.New()
	ids := make(sort.StringSlice, 0, len(unit.tasks))
	for id := range unit.tasks {
		ids = append(ids, id)
	}
	sort.Sort(ids)

	for _, id := range ids {
		_, _ = io.WriteString(hash, id)
	}

	unit.id = fmt.Sprintf("%x", hash.Sum(nil))
	return unit.id
}

// RankValue returns a point value for the tasks in the unit that can
// be used to compare units with eachother.
//
// Generally, higher point values are given to larger units and for
// units that have been in the queue for longer, with longer expected
// runtimes. The tasks priority act as a multiplying factor.
func (unit *Unit) RankValue() int64 {
	if unit.cachedValue > 0 {
		return unit.cachedValue
	}

	var (
		expectedRuntime time.Duration
		timeInQueue     time.Duration
		totalPriority   int64
		inCommitQueue   bool
		inPatch         bool
	)

	for _, t := range unit.tasks {
		if t.Requester == evergreen.MergeTestRequester {
			inCommitQueue = true
		} else if evergreen.IsPatchRequester(t.Requester) {
			inPatch = true
		}

		if !t.ScheduledTime.IsZero() {
			timeInQueue += time.Since(t.ScheduledTime)
		}

		totalPriority += t.Priority
		expectedRuntime += t.FetchExpectedDuration()
	}

	num := int64(len(unit.tasks))
	priority := 1 + (totalPriority / num)

	if inCommitQueue {
		priority += 100
	}

	if inPatch {
		unit.cachedValue += unit.distro.GetPatchZipperFactor()
	}

	unit.cachedValue += num
	unit.cachedValue += priority
	unit.cachedValue += priority * unit.distro.GetExpectedRuntimeFactor() * int64(math.Floor(expectedRuntime.Minutes()/float64(num)))
	unit.cachedValue += priority * unit.distro.GetTimeInQueueFactor() * int64(math.Floor(timeInQueue.Minutes()/float64(num)))
	return unit.cachedValue
}

// StringSet provides simple tools for managing sets of strings.
type StringSet map[string]struct{}

// Add places the string in the set.
func (s StringSet) Add(id string) { s[id] = struct{}{} }

// Check returns true if the string is a member of the set.
func (s StringSet) Check(id string) bool { _, ok := s[id]; return ok }

// Visit returns true if the string is already a member of the
// set. Otherwise it adds it to the set and returns false.
func (s StringSet) Visit(id string) bool {
	if s.Check(id) {
		return true
	}

	s.Add(id)
	return false
}

// TaskList implements sort.Interface on top of a slice of tasks. The
// provided sorting, orders members of task groups, and then
// prioritizes tasks by the number of dependencies, priority, and
// expected duration.
type TaskList []task.Task

func (tl TaskList) Len() int      { return len(tl) }
func (tl TaskList) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }
func (tl TaskList) Less(i, j int) bool {
	t1 := tl[i]
	t2 := tl[j]

	if t1.TaskGroupOrder != t2.TaskGroupOrder {
		return t1.TaskGroupOrder < t2.TaskGroupOrder
	}

	if len(t1.DependsOn) != len(t2.DependsOn) {
		return len(t1.DependsOn) < len(t2.DependsOn)
	}

	if t1.Priority != t2.Priority {
		return t1.Priority < t2.Priority
	}

	return t1.FetchExpectedDuration() < t2.FetchExpectedDuration()
}

// TaskPlan provides a sortable interface on top of a slice of
// schedulable units, with ordering of units provided by the
// implementation of RankValue.
type TaskPlan []*Unit

func (tpl TaskPlan) Len() int           { return len(tpl) }
func (tpl TaskPlan) Less(i, j int) bool { return tpl[i].RankValue() < tpl[j].RankValue() }
func (tpl TaskPlan) Swap(i, j int)      { tpl[i], tpl[j] = tpl[j], tpl[i] }

// PrepareTasksForPlanning takes a list of tasks for a distro and
// returns a TaskPlan, grouping tasks into the appropriate units.
func PrepareTasksForPlanning(distro *distro.Distro, tasks []task.Task) TaskPlan {
	cache := UnitCache{}
	groups := UnitCache{}
	versions := UnitCache{}

	for _, t := range tasks {
		var unit *Unit
		if t.TaskGroup != "" {
			unit = groups.Create(t.GetTaskGroupString(), t)
			cache.AddNew(t.Id, unit)
		} else {
			unit = cache.Create(t.Id, t)
		}
		unit.distro = distro
		versions.AddWhen(distro.ShouldGroupVersions(), t.Version, unit)

		// if it has dependencies:
		if len(t.DependsOn) > 0 {
			for _, dep := range t.DependsOn {
				cache.Create(dep.TaskId, t).distro = distro
			}
		}
	}

	return cache.Export()
}

// Export sorts the TaskPlan returning a unique list of tasks.
func (tpl TaskPlan) Export() []task.Task {
	sort.Sort(tpl)

	output := []task.Task{}
	seen := StringSet{}
	for _, unit := range tpl {
		tasks := unit.Export()
		sort.Sort(tasks)
		for _, t := range tasks {
			if seen.Visit(t.Id) {
				continue
			}

			output = append(output, t)
		}
	}

	return output
}
