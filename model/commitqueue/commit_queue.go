package commitqueue

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	triggerComment = "evergreen merge"
	PRPatchType    = "PR"
	CLIPatchType   = "CLI"
)

type Module struct {
	Module string `bson:"module" json:"module"`
	Issue  string `bson:"issue" json:"issue"`
}

func (m *Module) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(m) }
func (m *Module) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, m) }

type CommitQueueItem struct {
	Issue       string    `bson:"issue"`
	Version     string    `bson:"version,omitempty"`
	EnqueueTime time.Time `bson:"enqueue_time"`
	Modules     []Module  `bson:"modules"`
}

func (i *CommitQueueItem) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(i) }
func (i *CommitQueueItem) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, i) }

type CommitQueue struct {
	ProjectID  string            `bson:"_id"`
	Processing bool              `bson:"processing"`
	Queue      []CommitQueueItem `bson:"queue,omitempty"`
}

func (q *CommitQueue) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(q) }
func (q *CommitQueue) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, q) }

func InsertQueue(q *CommitQueue) error {
	return insert(q)
}

func (q *CommitQueue) Enqueue(item CommitQueueItem) (int, error) {
	position := q.FindItem(item.Issue)
	if !(position < 0) {
		return position + 1, errors.New("item already in queue")
	}

	if err := add(q.ProjectID, q.Queue, item); err != nil {
		return 0, errors.Wrapf(err, "can't add '%s' to queue '%s'", item.Issue, q.ProjectID)
	}

	item.EnqueueTime = time.Now()
	q.Queue = append(q.Queue, item)
	return len(q.Queue), nil
}

func (q *CommitQueue) Next() *CommitQueueItem {
	if len(q.Queue) == 0 {
		return nil
	}

	return &q.Queue[0]
}

func (q *CommitQueue) Remove(issue string) (bool, error) {
	itemIndex := q.FindItem(issue)
	if itemIndex < 0 {
		return false, nil
	}

	if err := remove(q.ProjectID, issue); err != nil {
		return false, errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)

	// clearing the front of the queue
	if itemIndex == 0 {
		if err := q.SetProcessing(false); err != nil {
			return false, errors.Wrap(err, "can't set processing to false")
		}
	}
	return true, nil
}

func (q *CommitQueue) UpdateVersion(item CommitQueueItem) error {
	return errors.Wrapf(addVersionID(q.ProjectID, item), "error updating version")
}

func (q *CommitQueue) FindItem(issue string) int {
	for i, queued := range q.Queue {
		if queued.Issue == issue {
			return i
		}
	}
	return -1
}

func (q *CommitQueue) SetProcessing(status bool) error {
	q.Processing = status
	if err := setProcessing(q.ProjectID, status); err != nil {
		return errors.Wrapf(err, "can't set processing on queue id '%s'", q.ProjectID)
	}

	return nil
}

func TriggersCommitQueue(commentAction string, comment string) bool {
	if commentAction == "deleted" {
		return false
	}
	return strings.HasPrefix(comment, triggerComment)
}

func ClearAllCommitQueues() (int, error) {
	clearedCount, err := clearAll()
	if err != nil {
		return 0, errors.Wrap(err, "can't clear queue")
	}

	return clearedCount, nil
}

func SetupEnv(env evergreen.Environment) error {
	if env == nil {
		return errors.New("no environment configured")
	}

	settings := env.Settings()
	githubToken, err := settings.GetGithubOauthToken()
	if err == nil && len(githubToken) > 0 {
		ctx, _ := env.Context()
		// Github PR Merge
		githubStatusSender, err := env.GetSender(evergreen.SenderGithubStatus)
		if err != nil {
			return errors.Wrap(err, "can't get github status sender")
		}
		sender, err := NewGithubPRLogger(ctx, "evergreen", githubToken, githubStatusSender)
		if err != nil {
			return errors.Wrap(err, "Failed to setup github merge logger")
		}
		if err = env.SetSender(evergreen.SenderGithubMerge, sender); err != nil {
			return errors.WithStack(err)
		}

		// Dequeue
		levelInfo := send.LevelInfo{
			Default:   level.Notice,
			Threshold: level.Notice,
		}
		sender, err = NewCommitQueueDequeueLogger("evergreen", levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup commit queue dequeue logger")
		}
		if err = env.SetSender(evergreen.SenderCommitQueueDequeue, sender); err != nil {
			return errors.WithStack(err)
		}

		evergreen.SetEnvironment(env)
	}

	return nil
}
