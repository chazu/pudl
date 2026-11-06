package checkreport

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chazu/pudl/internal/populate"
)

// An HTTP run's failed input names its request and error, without the exit
// status a command has.
func TestPopulateMarkdownNamesHTTPRequests(t *testing.T) {
	p := &Populate{Runs: 2, FailedInputs: []populate.FailedInput{
		{Run: 1, Item: "acme/b", Request: "GET /api/stacks/acme/b/export", ExitStatus: -1, Error: "HTTP 404 Not Found"},
	}}
	var b strings.Builder
	p.writeMarkdown(&b)
	out := b.String()
	assert.Contains(t, out, "  - acme/b: HTTP 404 Not Found\n")
	assert.Contains(t, out, "    request: GET /api/stacks/acme/b/export\n")
	assert.NotContains(t, out, "exit -1")
}
