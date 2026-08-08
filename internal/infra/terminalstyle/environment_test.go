package terminalstyle

import "testing"

func TestNoColorRequestedUsesPresenceOnly(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if !NoColorRequested() {
		t.Fatal("empty NO_COLOR should disable terminal styles")
	}
	t.Setenv("NO_COLOR", "0")
	if !NoColorRequested() {
		t.Fatal("NO_COLOR value must not re-enable terminal styles")
	}
}
