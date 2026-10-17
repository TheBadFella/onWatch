package web

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// LOW 3: reportPreferredHeight must not let a first report that lands before
// the native bridge / iframe host is ready suppress every later same-height
// report. The debounce only applies once a report has actually been delivered,
// and visibilitychange/focus re-arm it.
func TestMenubarResizeReportRetriesUntilDelivered(t *testing.T) {
	page, err := staticFS.ReadFile("static/menubar.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)

	// The re-arm wiring must be present, or a host that missed the first report
	// stays stale forever.
	for _, needle := range []string{
		`document.addEventListener("visibilitychange"`,
		`window.addEventListener("focus"`,
		`resizeReportDelivered = false`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("menubar.html missing resize re-arm wiring %q", needle)
		}
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required for menubar resize test")
	}

	start := strings.Index(html, "let resizeReportDelivered = false;")
	end := strings.Index(html, "function ensureMultiProviderSelection()")
	if start < 0 || end < start {
		t.Fatal("resize report logic not found in menubar.html")
	}
	slice := html[start:end]

	test := `
const assert = require('node:assert/strict');
let lastReportedHeight = null;
const measured = 300;
function measuredContentHeight() { return measured; }
function preferredResizeFloor() { return 140; }
function preferredResizeCeiling() { return 100000; }
const windowPosts = [];
const delivered = [];
global.window = {
  postMessage: (payload) => windowPosts.push(payload),
  parent: null,
  webkit: undefined,
};
` + slice + `
// No host channel yet: a report fires but is not delivered, and the debounce
// stays disabled so a later same-height report is not suppressed.
reportPreferredHeight();
assert.equal(delivered.length, 0, 'no host channel should mean no delivery');
assert.equal(windowPosts.length, 1, 'the report should still be attempted');

// The host becomes ready (iframe parent listener attached).
window.parent = { postMessage: (payload) => delivered.push(payload.height) };
reportPreferredHeight();
assert.equal(delivered.length, 1, 'a same-height report must retry once a host appears');
assert.equal(delivered[0], 300);

// Now that a report was delivered, the 2px debounce applies again.
reportPreferredHeight();
assert.equal(delivered.length, 1, 'a delivered same-height report must be debounced');

// Re-arming (what visibilitychange/focus does) lets the same height through.
resizeReportDelivered = false;
reportPreferredHeight();
assert.equal(delivered.length, 2, 're-armed report must be delivered again');
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node)
	cmd.Stdin = strings.NewReader(test)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
}
