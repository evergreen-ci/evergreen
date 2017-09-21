package gimlet

var jobIDSource <-chan int

func init() {
	jobIDSource = func() <-chan int {
		out := make(chan int, 50)
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

// getNumber is a source of safe monotonically increasing integers
// for use in request ids.
func getNumber() int {
	return <-jobIDSource
}
