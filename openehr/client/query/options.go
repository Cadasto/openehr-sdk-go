package query

// executeConfig holds resolved per-call options.
type executeConfig struct {
	offset int
	fetch  int
	ehrID  string
}

// ExecuteOption mutates ad-hoc or stored query execution.
type ExecuteOption func(*executeConfig)

// WithOffset sets the 0-based row offset.
func WithOffset(n int) ExecuteOption {
	return func(c *executeConfig) { c.offset = n }
}

// WithFetch limits the number of rows returned.
func WithFetch(n int) ExecuteOption {
	return func(c *executeConfig) { c.fetch = n }
}

// WithEHRID scopes the query to one EHR (population vs single-EHR).
func WithEHRID(id string) ExecuteOption {
	return func(c *executeConfig) { c.ehrID = id }
}
