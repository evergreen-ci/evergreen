// Error Handler
//
// The task runner is responsible for downloading agents onto host
// machines and starting them. As this requires synchronous SSH
// connections and the process is error prone if SSHd is slow to
// start, there's an agent already running on the host, network
// connectivity is disrupted, or there are too many outbound SSH
// connections from the runner box.
//
// These problems are frequent and real, but not particularly
// actionable, and given that the taskrunner operation runs multiple
// times a minute in normal operation, the reports of problems in
// isolation are particularly non-actionable.
//
// Simply collecting the errors per-taskrunner run, is not sufficen

package taskrunner

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

var errorCollector *errorCollectorImpl

func init() {
	errorCollector = newErrorCollector()
}

func newErrorCollector() *errorCollectorImpl {
	return &errorCollectorImpl{
		cache: make(map[hostRecord]errorRecord),
	}
}

type errorCollectorImpl struct {
	cache map[hostRecord]errorRecord
	mutex sync.Mutex
}

type hostRecord struct {
	id       string
	distro   string
	provider string
}

type errorRecord struct {
	count  int
	host   *host.Host
	errors []string
}

func (c *errorCollectorImpl) add(h *host.Host, err error) {
	rec := hostRecord{
		id:       h.Id,
		distro:   h.Distro.Id,
		provider: h.Distro.Provider,
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	doc, ok := c.cache[rec]

	if err == nil {
		if ok {
			grip.Info(message.Fields{
				"message":  "host recovered after previous errors",
				"host":     rec.id,
				"distro":   rec.distro,
				"provider": rec.provider,
				"runner":   RunnerName,
				"failures": doc.count,
			})
			delete(c.cache, rec)
		}

		return
	}

	doc.host = h
	doc.errors = append(doc.errors, err.Error())
	doc.count++
	c.cache[rec] = doc
}

func processErrorItem(rec hostRecord, errors errorRecord) string {
	lines := []string{"",
		fmt.Sprintf("Host: '%s'", rec.id),
		fmt.Sprintf("Provider: '%s'", rec.provider),
		fmt.Sprintf("Distro: '%s'", rec.distro),
		fmt.Sprintf("Consecutive Failures: %d", errors.count),
	}

	if rec.provider == evergreen.ProviderNameStatic {
		env := evergreen.GetEnvironment()
		queue := env.LocalQueue()

		lines = append(lines, "Action: Disabled Host")
		err := errors.host.DisablePoisonedHost()

		job := units.NewDecoHostNotifyJob(env, errors.host, err,
			"host encountered consecutive set up failures")
		grip.Critical(queue.Put(job))
	}

	errorLines := strings.Split(strings.Join(errors.errors, "\n"), "\n")
	lines = append(lines, "\n\n\t"+strings.Join(errorLines, "\n\t"))

	return strings.Join(lines, "\n")
}

func (c *errorCollectorImpl) report() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	reports := []string{}

	for rec, errors := range c.cache {
		if errors.count >= 10 {
			reports = append(reports, processErrorItem(rec, errors))
			delete(c.cache, rec)
		}
	}

	if len(reports) > 0 {
		return errors.New(strings.Join(reports,
			"\n\n------------------------------------------------------------------------\n\n"))
	}

	return nil
}
