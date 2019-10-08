package main // import "github.com/nutmegdevelopment/sumologic/journalstream"

import (
	"flag"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/go-systemd/sdjournal"
	"github.com/nutmegdevelopment/sumologic/buffer"
	"github.com/nutmegdevelopment/sumologic/upload"
)

var (
	bTime     int
	url       string
	nameField string
	window    int64
	bSize     = 4096
	uploader  upload.Uploader
)

func init() {
	flag.StringVar(&url, "u", "http://localhost", "URL of sumologic collector")
	flag.StringVar(&nameField, "n", "_SYSTEMD_UNIT", "Journald field to use as log name")
	flag.IntVar(&bTime, "b", 3, "Maximum time to buffer messages before upload")
	flag.Int64Var(&window, "w", 30, "Time Window. Maximum age (in seconds) of log entries to relay.")
	debug := flag.Bool("d", false, "Debug mode")
	flag.Parse()

	if *debug {
		buffer.DebugLogging()
		upload.DebugLogging()
		log.SetLevel(log.DebugLevel)
	}
}

func watch(eventCh chan<- *sdjournal.JournalEntry, quitChan <-chan bool) {
	j, err := sdjournal.NewJournalFromDir("/var/log/journal")
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Journald watcher started")

	err = j.SeekTail()
	if err != nil {
		log.Fatal("Unable to seek to end of journal")
	}
	log.Debug("Journal seek completed")

	defer close(eventCh)

	for {
		select {
		case <-quitChan:
			return
		default:
			n, err := j.Next()
			if err != nil {
				log.Fatal(err)
			}

			if n == 0 {
				// At the end of the journal, wait for new entries
				log.Debug("Waiting for new journal entries")
				ret := j.Wait(300 * time.Second)
				if ret < 0 {
					log.Fatalf("Error waiting for journal entries: %d", ret)
				}
				if ret == sdjournal.SD_JOURNAL_NOP {
					log.Warn("No journal entries for 300 seconds")
				}
				continue
			}

			entry, err := j.GetEntry()
			if err != nil {
				log.Error(err)
				continue
			}
			eventCh <- entry

		}
	}
}

func parse(eventCh <-chan *sdjournal.JournalEntry, buf *buffer.Buffer, quitChan <-chan bool) {
	log.Debug("Journald parser started")
	for {
		select {
		case <-quitChan:
			return

		case event := <-eventCh:
			// Check if event within window
			eventTime := time.Unix(
				int64(event.RealtimeTimestamp/1000000),
				int64(event.RealtimeTimestamp%1000000*1000))

			if eventTime.After(time.Now().Add(-time.Second * time.Duration(window))) {

				msg := eventTime.String() + " "

				if _, ok := event.Fields["MESSAGE"]; !ok {
					// Ignore event, no message
					continue
				}

				msg += event.Fields["MESSAGE"]

				// Set name field
				name := "UNDEFINED"
				if _, ok := event.Fields[nameField]; ok {
					name = event.Fields[nameField]
				}

				buf.Add([]byte(msg), name)

			}

		}
	}
}

func main() {
	uploader = upload.NewUploader(url)
	buf := buffer.NewBuffer(bSize)

	quitCh := make(chan bool, 1)
	eventCh := make(chan *sdjournal.JournalEntry, 1024)

	go watch(eventCh, quitCh)

	go parse(eventCh, buf, quitCh)

	for {
		time.Sleep(time.Second * time.Duration(bTime))
		err := buf.Send(uploader)
		if err != nil {
			log.Error(err)
		}
	}
}
