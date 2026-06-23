package query

// executeConfig holds resolved per-call options. The *Set flags track
// whether the caller supplied an explicit value, so an explicit
// WithOffset(0) / WithFetch(0) is distinguishable from "unset" and can be
// emitted on the wire.
type executeConfig struct {
	offset    int
	offsetSet bool
	fetch     int
	fetchSet  bool
	ehrID     string
	useGET    bool
}

// ExecuteOption mutates ad-hoc or stored query execution.
type ExecuteOption func(*executeConfig)

// WithOffset sets the 0-based row offset. An explicit WithOffset(0) is
// honoured (sent on the wire), not silently dropped.
func WithOffset(n int) ExecuteOption {
	return func(c *executeConfig) { c.offset = n; c.offsetSet = true }
}

// WithFetch limits the number of rows returned. An explicit WithFetch(0)
// requests zero rows; when unset the server applies its implementation-
// defined default (the openEHR Fetch schema has no fixed default).
func WithFetch(n int) ExecuteOption {
	return func(c *executeConfig) { c.fetch = n; c.fetchSet = true }
}

// WithEHRID scopes the query to one EHR (population vs single-EHR).
func WithEHRID(id string) ExecuteOption {
	return func(c *executeConfig) { c.ehrID = id }
}

// WithGET issues the query via GET instead of POST, passing q / offset /
// fetch / ehr_id / query_parameters as URL query parameters (the spec's
// GET variants of the query endpoints). POST (the default) is recommended
// for long AQL since GET is subject to URL-length limits; use GET for
// short, cacheable queries.
func WithGET() ExecuteOption {
	return func(c *executeConfig) { c.useGET = true }
}
