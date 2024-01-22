/*
Log Chunk

A log chunk is a storage abstraction representing a sequential, continuous
section of a single log. Logs can be coherently represented by a set of ordered
chunks.
*/
package log

// chunkInfo represents a log chunk file's metadata that enables optimized
// fetching of log files stored as a set of chunks in pail-backed bucket
// storage.
type chunkInfo struct {
	key      string
	numLines int
	start    int64
	end      int64
}

// chunkGroup represents a set of chunks belonging to a single log.
type chunkGroup struct {
	name   string
	chunks []chunkInfo
}
