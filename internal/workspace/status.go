package workspace

// IsRunning reports whether every process declared by an instance is alive.
func IsRunning(state RuntimeState) bool {
	for _, service := range state.Services {
		if service.PID > 0 && !processAlive(service.PID) {
			return false
		}
	}
	return true
}
