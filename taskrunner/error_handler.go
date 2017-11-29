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
	errors []string
}

func (c *errorCollectorImpl) add(id, distro, provider string, err error) {
	rec := hostRecord{
		id:       id,
		distro:   distro,
		provider: provider,
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	doc, ok := c.cache[rec]

	if err == nil {
		if ok {
			grip.Info(message.Fields{
				"message":  "host recovered after previous errors",
				"host":     id,
				"distro":   distro,
				"provider": provider,
				"runner":   RunnerName,
				"failures": doc.count,
			})
			delete(c.cache, rec)
		}

		return
	}

	doc.errors = append(doc.errors, err.Error())
	doc.count++
	c.cache[rec] = doc
}

func errorItem(rec hostRecord, errors errorRecord) string {
	errorLines := strings.Split(strings.Join(errors.errors, "\n"), "\n")
	return strings.Join([]string{
		fmt.Sprintf("Host: '%s'", rec.id),
		fmt.Sprintf("Provider: '%s'", rec.provider),
		fmt.Sprintf("Distro: '%s'", rec.distro),
		fmt.Sprintf("Consecutive Failures: %d", errors.count),
		"\n\n\t" + strings.Join(errorLines, "\n\t"),
	}, "\n")
}

func (c *errorCollectorImpl) report() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	reports := []string{}

	for rec, errors := range c.cache {
		if errors.count < 5 {
			continue
		}
		reports = append(reports, errorItem(rec, errors))
		delete(c.cache, rec)
	}

	if len(reports) > 0 {
		return errors.New(strings.Join(reports,
			"\n\n------------------------------------------------------------------------\n\n"))
	}

	return nil
}
