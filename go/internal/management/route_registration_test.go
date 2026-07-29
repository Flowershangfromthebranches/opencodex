package management

import (
	"testing"
)

// Every route the API answers has to be registered on the mux too. A handler
// that exists but is never routed answers 404 in production while looking
// perfectly wired in the source -- which is exactly how the dashboard's
// Restart button went dead.
func TestRestartRouteIsRegisteredOnTheMux(t *testing.T) {
	var found bool
	for _, route := range RegisteredRoutes() {
		if route == "POST /api/system/restart" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("POST /api/system/restart is handled but not registered, so it answers 404")
	}
}
