package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

const region string = "us-east-1"

type sqsFIFOQueue struct {
	sqsClient  *sqs.SQS
	sqsURL     string
	id         string
	started    bool
	numRunning int
	tasks      struct { // map jobID to job information
		completed map[string]bool
		all       map[string]amboy.Job
	}
	runner amboy.Runner
	mutex  sync.RWMutex
}

// NewSQSFifoQueue constructs a AWS SQS backed Queue
// implementation. This queue, generally is ephemeral: tasks are
// removed from the queue, and therefore may not handle jobs across
// restarts.
func NewSQSFifoQueue(queueName string, workers int) (amboy.Queue, error) {
	q := &sqsFIFOQueue{
		sqsClient: sqs.New(session.Must(session.NewSession(&aws.Config{
			Region: aws.String(region),
		}))),
		id: fmt.Sprintf("queue.remote.sqs.fifo..%s", uuid.NewV4().String()),
	}
	q.tasks.completed = make(map[string]bool)
	q.tasks.all = make(map[string]amboy.Job)
	q.runner = pool.NewLocalWorkers(workers, q)
	result, err := q.sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(fmt.Sprintf("%s.fifo", queueName)),
		Attributes: map[string]*string{
			"FifoQueue": aws.String("true"),
		},
	})
	if err != nil {
		return nil, errors.Errorf("Error creating queue: %s", err)
	}
	q.sqsURL = *result.QueueUrl
	return q, nil
}

func (q *sqsFIFOQueue) ID() string {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.id
}

func (q *sqsFIFOQueue) Put(ctx context.Context, j amboy.Job) error {
	name := j.ID()

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job timeinfo")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if !q.Started() {
		return errors.Errorf("cannot put job %s; queue not started", name)
	}

	if _, ok := q.tasks.all[name]; ok {
		return errors.Errorf("cannot add %s because duplicate job already exists", name)
	}

	dedupID := strings.Replace(j.ID(), " ", "", -1) //remove all spaces
	curStatus := j.Status()
	curStatus.ID = dedupID
	j.SetStatus(curStatus)
	jobItem, err := registry.MakeJobInterchange(j, amboy.JSON)
	if err != nil {
		return errors.Wrap(err, "Error converting job in Put")
	}
	job, err := json.Marshal(jobItem)
	if err != nil {
		return errors.Wrap(err, "Error marshalling job to JSON in Put")
	}
	input := &sqs.SendMessageInput{
		MessageBody:            aws.String(string(job)),
		QueueUrl:               aws.String(q.sqsURL),
		MessageGroupId:         aws.String(randomString(16)),
		MessageDeduplicationId: aws.String(dedupID),
	}
	_, err = q.sqsClient.SendMessageWithContext(ctx, input)

	if err != nil {
		return errors.Wrap(err, "Error sending message in Put")
	}
	q.tasks.all[name] = j
	return nil
}

func (q *sqsFIFOQueue) Save(ctx context.Context, j amboy.Job) error {
	name := j.ID()

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if !q.Started() {
		return errors.Errorf("cannot save job %s; queue not started", name)
	}

	if _, ok := q.tasks.all[name]; !ok {
		return errors.Errorf("cannot save '%s' because a job does not exist with that name", name)
	}

	q.tasks.all[name] = j
	return nil

}

// Returns the next job in the queue. These calls are
// blocking, but may be interrupted with a canceled context.
func (q *sqsFIFOQueue) Next(ctx context.Context) amboy.Job {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if ctx.Err() != nil {
		return nil
	}

	messageOutput, err := q.sqsClient.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(q.sqsURL),
	})
	if err != nil || len(messageOutput.Messages) == 0 {
		grip.Debugf("No messages received in Next from %s", q.sqsURL)
		return nil
	}
	message := messageOutput.Messages[0]
	_, err = q.sqsClient.DeleteMessageWithContext(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.sqsURL),
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		return nil
	}

	var jobItem *registry.JobInterchange
	err = json.Unmarshal([]byte(*message.Body), jobItem)
	if err != nil {
		return nil
	}
	job, err := jobItem.Resolve(amboy.JSON)
	if err != nil {
		return nil
	}

	if job.TimeInfo().IsStale() {
		return nil
	}

	q.numRunning++
	return job
}

func (q *sqsFIFOQueue) Get(ctx context.Context, name string) (amboy.Job, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	j, ok := q.tasks.all[name]
	return j, ok
}

// true if queue has started dispatching jobs
func (q *sqsFIFOQueue) Started() bool {
	return q.started
}

// Used to mark a Job complete and remove it from the pending
// work of the queue.
func (q *sqsFIFOQueue) Complete(ctx context.Context, job amboy.Job) {
	if ctx.Err() != nil {
		return
	}
	name := job.ID()
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if ctx.Err() != nil {
		grip.Notice(message.Fields{
			"message":   "Did not complete job because context cancelled",
			"id":        name,
			"operation": "Complete",
		})
		return
	}
	q.tasks.completed[name] = true
	savedJob := q.tasks.all[name]
	if savedJob != nil {
		savedJob.SetStatus(job.Status())
		savedJob.UpdateTimeInfo(job.TimeInfo())
	}

}

// Returns a channel that produces completed Job objects.
func (q *sqsFIFOQueue) Results(ctx context.Context) <-chan amboy.Job {
	results := make(chan amboy.Job)

	go func() {
		q.mutex.RLock()
		defer q.mutex.RUnlock()
		defer close(results)
		defer recovery.LogStackTraceAndContinue("cancelled context in Results")
		for name, job := range q.tasks.all {
			if _, ok := q.tasks.completed[name]; ok {
				select {
				case <-ctx.Done():
					return
				case results <- job:
					continue
				}
			}
		}
	}()
	return results
}

// Returns a channel that produces the status objects for all
// jobs in the queue, completed and otherwise.
func (q *sqsFIFOQueue) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	allInfo := make(chan amboy.JobStatusInfo)
	go func() {
		q.mutex.RLock()
		defer q.mutex.RUnlock()
		defer close(allInfo)
		for _, job := range q.tasks.all {
			select {
			case <-ctx.Done():
				return
			case allInfo <- job.Status():
			}

		}
	}()
	return allInfo
}

// Returns an object that contains statistics about the
// current state of the Queue.
func (q *sqsFIFOQueue) Stats(ctx context.Context) amboy.QueueStats {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	s := amboy.QueueStats{
		Completed: len(q.tasks.completed),
	}
	s.Running = q.numRunning - s.Completed

	output, err := q.sqsClient.GetQueueAttributesWithContext(ctx,
		&sqs.GetQueueAttributesInput{
			AttributeNames: []*string{aws.String("ApproximateNumberOfMessages"),
				aws.String("ApproximateNumberOfMessagesNotVisible")},
			QueueUrl: aws.String(q.sqsURL),
		})
	if err != nil {
		return s
	}

	numMsgs, _ := strconv.Atoi(*output.Attributes["ApproximateNumberOfMessages"])                   // nolint
	numMsgsInFlight, _ := strconv.Atoi(*output.Attributes["ApproximateNumberOfMessagesNotVisible"]) // nolint

	s.Pending = numMsgs + numMsgsInFlight
	s.Total = s.Pending + s.Completed

	return s
}

// Getter for the Runner implementation embedded in the Queue
// instance.
func (q *sqsFIFOQueue) Runner() amboy.Runner {
	return q.runner
}

func (q *sqsFIFOQueue) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot change runners after starting")
	}

	q.runner = r
	return nil
}

// Begins the execution of the job Queue, using the embedded
// Runner.
func (q *sqsFIFOQueue) Start(ctx context.Context) error {
	if q.Started() {
		return nil
	}

	q.started = true
	err := q.runner.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting runner")
	}
	return nil
}
