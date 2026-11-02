package uiserver

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

// readEvents returns the data of each "version" event, until n arrive.
func readEvents(t *testing.T, sc *bufio.Scanner, n int) []string {
	t.Helper()
	var out []string
	event := ""
	for len(out) < n && sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "version":
			out = append(out, strings.TrimPrefix(line, "data: "))
		}
	}
	return out
}

func TestEventsPushVersionChangesAndStopOnShutdown(t *testing.T) {
	var version atomic.Value
	version.Store("v1")
	h := Handler(Options{
		Assets:        fstest.MapFS{},
		Stamp:         func(context.Context) (string, error) { return version.Load().(string), nil },
		EventInterval: 20 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, h) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	sc := bufio.NewScanner(resp.Body)
	require.Equal(t, []string{"v1"}, readEvents(t, sc, 1), "the current version on connect")

	version.Store("v2")
	require.Equal(t, []string{"v2"}, readEvents(t, sc, 1), "a change is pushed")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "shutdown is not held up by an open stream")
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return with an event stream open")
	}
}

func TestEventsWithoutStampIs404(t *testing.T) {
	require.Equal(t, http.StatusNotFound, get(t, testHandler(nil), "localhost", "/api/events").Code)
}
