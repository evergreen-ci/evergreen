package commitqueue

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
)

type DequeueItem struct {
	ProjectID string
	Item      string
	Status    string
}

func (d *DequeueItem) String() string {
	return fmt.Sprintf("dequeue commit queue '%s' item '%s'", d.ProjectID, d.Item)
}

func (d *DequeueItem) Send() error {
	queue, err := FindOneId(d.ProjectID)
	if err != nil {
		return errors.Wrapf(err, "no matching commit queue for project '%s'", d.ProjectID)
	}
	if queue == nil {
		return errors.Errorf("no queue found for project '%s'", d.ProjectID)
	}

	foundItem, err := queue.Remove(d.Item)
	if err != nil {
		return errors.Wrap(err, "can't remove item")
	}
	if foundItem == nil {
		return errors.Errorf("item '%s' not found on queue '%s'", d.Item, d.ProjectID)
	}

	status := evergreen.MergeTestSucceeded
	if d.Status == evergreen.PatchFailed {
		status = evergreen.MergeTestFailed
	}
	version := foundItem.Version
	if version == "" {
		version = foundItem.Issue
	}
	if version == "" {
		return errors.Errorf("item %s in queue %s has no version", d.Item, d.ProjectID)
	}
	event.LogCommitQueueConcludeTest(version, status)

	if err = queue.SetProcessing(false); err != nil {
		return errors.Wrap(err, "can't set processing to false")
	}

	return nil
}

func (d *DequeueItem) Valid() bool {
	return len(d.ProjectID) != 0 && len(d.Item) != 0 && len(d.Status) != 0
}
