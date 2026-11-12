package notify

import (
	"strings"
	"testing"
)

// Only an early reset is worth flagging. A reset with no schedule to compare
// against is reported as a new window, never as "cause unknown".
func TestResetCauseLabel_OnlyEarlyResetsAreFlagged(t *testing.T) {
	t.Parallel()
	for _, cause := range []string{"scheduled", "banked_credit", "unknown", ""} {
		if got := resetCauseLabel(cause); strings.Contains(strings.ToLower(got), "unknown") {
			t.Errorf("resetCauseLabel(%q) = %q, must not say unknown", cause, got)
		}
	}
	if got := resetCauseLabel("early_unknown"); !strings.Contains(got, "Early reset") {
		t.Errorf("early reset label = %q", got)
	}
	if got := resetMessage(QuotaStatus{}); strings.Contains(got, "not recorded") {
		t.Errorf("reset without evidence reads as missing data: %q", got)
	}
}
