package main // import "github.com/nutmegdevelopment/sumologic/filestream"

import (
	"flag"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/hpcloud/tail"
	"github.com/nutmegdevelopment/sumologic/buffer"
	"github.com/nutmegdevelopment/sumologic/upload"
)

var (
	fileName string
	bTime    int
	url      string
	sendName string
	bSize    = 4096
	uploader upload.Uploader
)

func init() {
	flag.StringVar(&fileName, "f", "", "File to stream")
	flag.StringVar(&url, "u", "http://localhost", "URL of sumologic collector")
	flag.StringVar(&sendName, "n", "", "Name to send to Sumologic")
	flag.IntVar(&bTime, "b", 3, "Maximum time to buffer messages before upload")
	debug := flag.Bool("d", false, "Debug mode")
	flag.Parse()

	if *debug {
		buffer.DebugLogging()
		upload.DebugLogging()
		log.SetLevel(log.DebugLevel)
	}
}

func watchFile(b *buffer.Buffer, file string) (err error) {
	t, err := tail.TailFile(file, tail.Config{
		Follow:    true,
		MustExist: true,
		ReOpen:    false,
		Poll:      true,
	})
	if err != nil {
		return
	}
	defer t.Cleanup()
	for line := range t.Lines {
		b.Add([]byte(line.Text), sendName)
	}
	err = t.Wait()
	return err
}

func main() {
	buf := buffer.NewBuffer(bSize)
	uploader = upload.NewUploader(url)
	quit := make(chan bool)

	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				time.Sleep(time.Second * time.Duration(bTime))
				buf.Send(uploader)
			}
		}
	}()

	err := watchFile(buf, fileName)
	quit <- true
	log.Fatal(err)
}
