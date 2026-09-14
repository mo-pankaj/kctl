// Package logview implements kctl's log tailing view.
package logview

// Ring is a fixed-capacity FIFO of log lines.
//
// Logs are unbounded and a long tail on a chatty pod would otherwise grow
// memory without limit. When the buffer fills, the oldest line is dropped and
// Truncated reports it, so the loss is visible in the header rather than silent.
type Ring struct {
	buf       []string
	start     int
	length    int
	truncated bool
}

// NewRing builds a ring holding at most capacity lines. A capacity below one is
// clamped to one.
func NewRing(capacity int) (r *Ring) {
	if capacity < 1 {
		capacity = 1
	}

	r = &Ring{buf: make([]string, capacity)}
	return r
}

// Add appends a line, dropping the oldest when full.
func (r *Ring) Add(line string) {
	capacity := len(r.buf)

	if r.length < capacity {
		r.buf[(r.start+r.length)%capacity] = line
		r.length++
		return
	}

	r.buf[r.start] = line
	r.start = (r.start + 1) % capacity
	r.truncated = true
}

// Lines returns the buffered lines, oldest first, as a copy.
func (r *Ring) Lines() (lines []string) {
	capacity := len(r.buf)
	lines = make([]string, 0, r.length)

	for i := 0; i < r.length; i++ {
		lines = append(lines, r.buf[(r.start+i)%capacity])
	}

	return lines
}

// Len returns the number of buffered lines.
func (r *Ring) Len() (n int) {
	n = r.length
	return n
}

// Truncated reports whether any line has been dropped.
func (r *Ring) Truncated() (dropped bool) {
	dropped = r.truncated
	return dropped
}
