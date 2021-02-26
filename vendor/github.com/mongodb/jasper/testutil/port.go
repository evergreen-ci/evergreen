package testutil

var intSource <-chan int

func init() {
	intSource = func() <-chan int {
		out := make(chan int, 25)
		go func() {
			id := 3000
			for {
				id++
				out <- id
			}
		}()
		return out
	}()
}

// GetPortNumber returns a new port number that has not been used in the current
// runtime.
func GetPortNumber() int {
	return <-intSource
}
