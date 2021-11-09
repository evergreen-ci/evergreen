package job

var jobIDSource <-chan int

func init() {
	jobIDSource = func() <-chan int {
		out := make(chan int, 10)
		go func() {
			var jobID int
			for {
				jobID++
				out <- jobID
			}
		}()
		return out
	}()
}

// GetNumber is a source of safe monotonically increasing integers
// for use in Job ids.
func GetNumber() int {
	return <-jobIDSource
}
