package smart

import "github.com/cadasto/openehr-sdk-go/auth"

// HeldTokens returns the access and refresh tokens the source holds, so a
// test can check that a failed exchange or refresh left them as they were.
func (s *Source) HeldTokens() (auth.Token, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur, s.refresh
}

// Settings returns the configuration the source keeps, so a test can check
// that a change the caller makes afterwards to what it passed does not reach
// the source.
func (s *Source) Settings() Config { return s.cfg }
