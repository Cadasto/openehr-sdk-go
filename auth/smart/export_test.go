package smart

import "github.com/cadasto/openehr-sdk-go/auth"

// HeldTokens returns the access and refresh tokens the source holds, so a
// test can check that a failed exchange or refresh left them as they were.
func (s *Source) HeldTokens() (auth.Token, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur, s.refresh
}
