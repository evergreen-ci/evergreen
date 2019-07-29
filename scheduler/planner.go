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

type Unit struct {
	tasks       taskUnitList
	cachedValue int64
	id          string
}

func NewUnit(t task.Task) *Unit {
	return &Unit{
		tasks: []task.Task{t},
	}
}

type UnitCache map[string]*Unit

func NewUnitCache() UnitCache { return map[string]*Unit{} }

func (cache UnitCache) AddWhen(cond bool, id string, unit *Unit) {
	if !cond {
		return
	}

	cache.AddNew(id, unit)
}

func (cache UnitCache) AddNew(id string, unit *Unit) {
	if existing, ok := cache[id]; ok {
		for _, t := range unit.tasks {
			existing.Add(t)
		}

		return
	}

	cache[id] = unit
}

func (cache UnitCache) Create(id string, t task.Task) *Unit {
	if unit, ok := cache[id]; ok {
		unit.Add(t)
		return unit
	}

	unit := NewUnit(t)
	cache.AddNew(id, unit)
	return unit
}

func (cache UnitCache) Export() TaskPlan {
	seen := map[string]struct{}{}
	tpl := TaskPlan{}
	for id := range cache {
		hash := cache[id].ID()
		if _, ok := seen[hash]; ok {
			continue
		}

		seen[hash] = struct{}{}
		tpl = append(tpl, cache[id])
	}

	return tpl
}

func (unit *Unit) Add(t task.Task) {
	seen := false
	for _, et := range unit.tasks {
		if et.Id == t.Id {
			seen = true
		}
	}

	if !seen {
		unit.tasks = append(unit.tasks, t)
	}
}

func (unit *Unit) ID() string {
	if unit.id != "" {
		return unit.id
	}

	hash := sha1.New()
	ids := make(sort.StringSlice, len(unit.tasks))
	for idx, t := range unit.tasks {
		ids[idx] = t.Id
	}
	sort.Sort(ids)

	for _, id := range ids {
		_, _ = io.WriteString(hash, id)
	}

	unit.id = fmt.Sprintf("%x", hash.Sum(nil))
	return unit.id
}

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
		timeInQueue += time.Since(t.ScheduledTime)
		totalPriority += t.Priority
		expectedRuntime += t.FetchExpectedDuration()
	}

	num := int64(len(unit.tasks))
	priority := totalPriority / num

	if inCommitQueue {
		priority += 100
	}

	if inPatch {
		priority += 10
	}

	unit.cachedValue = num * priority
	unit.cachedValue += priority * (int64(math.Floor(expectedRuntime.Minutes())) / num)
	unit.cachedValue += priority * (int64(math.Floor(timeInQueue.Minutes())) / num)

	return unit.cachedValue
}

type taskUnitList []task.Task

func (tl taskUnitList) Len() int      { return len(tl) }
func (tl taskUnitList) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }
func (tl taskUnitList) Less(i, j int) bool {
	t1 := tl[i]
	t2 := tl[j]

	if t1.TaskGroupOrder != t2.TaskGroupOrder {
		return t1.TaskGroupOrder < t2.TaskGroupOrder
	}

	if len(t1.DependsOn) != t2.TaskGroupOrder {
		return len(t1.DependsOn) < len(t2.DependsOn)
	}

	if t1.Priority != t2.Priority {
		return t1.Priority < t2.Priority
	}

	return t1.FetchExpectedDuration() < t2.FetchExpectedDuration()
}

type TaskPlan []*Unit

func (tpl TaskPlan) Len() int           { return len(tpl) }
func (tpl TaskPlan) Less(i, j int) bool { return tpl[i].RankValue() < tpl[i].RankValue() }
func (tpl TaskPlan) Swap(i, j int)      { tpl[i], tpl[j] = tpl[j], tpl[i] }

func PrepareTasksForPlanning(tasks []task.Task) TaskPlan {
	cache := NewUnitCache()
	groups := NewUnitCache()
	versions := NewUnitCache()

	// TODO this should be passed in somehow:
	distroCache := map[string]*distro.Distro{}

	for _, t := range tasks {
		// if its in a taskgroup:
		var unit *Unit
		if t.TaskGroup != "" {
			unit = groups.Create(t.GetTaskGroupString(), t)
			cache.AddNew(t.Id, unit)
		} else {
			unit = cache.Create(t.Id, t)
		}

		versions.AddWhen(distroCache[t.DistroId].ShouldGroupVersions(), t.Version, unit)

		// if it has dependencies:
		if len(t.DependsOn) > 0 {
			for _, dep := range t.DependsOn {
				cache.Create(dep.TaskId, t)
			}
		}
	}

	return cache.Export()
}

func (tpl TaskPlan) Export() []task.Task {
	sort.Sort(tpl)

	tracked := map[string]struct{}{}
	output := []task.Task{}

	for _, unit := range tpl {
		sort.Sort(unit.tasks)
		for _, t := range unit.tasks {
			if _, ok := tracked[t.Id]; ok {
				continue
			}

			tracked[t.Id] = struct{}{}
			output = append(output, t)
		}
	}

	return output
}
