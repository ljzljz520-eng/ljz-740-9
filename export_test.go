package gosr

// resetDefaultForTest clears the lazily-loaded default backend so each test
// can point GOSR_NATIVE_LIB elsewhere.
func resetDefaultForTest() {
	defaultMu.Lock()
	if defaultBackend != nil {
		_ = defaultBackend.Close()
	}
	defaultBackend = nil
	defaultMu.Unlock()
}
