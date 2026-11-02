package uiserver

import (
	"fmt"
	"net/http"
	"time"
)

const heartbeatEvery = 15 * time.Second

// streamVersions sends the change stamp as Server-Sent Events: once on
// connect, then whenever it changes. The catalog is SQLite written by other
// pudl processes and cannot notify anyone, so the stamp is checked every
// interval; the browser is pushed only real changes. A comment line every
// 15s keeps proxies from closing an idle stream.
func streamVersions(w http.ResponseWriter, r *http.Request, stamp StampFunc, interval time.Duration) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if interval <= 0 {
		interval = time.Second
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	last := ""
	send := func() bool {
		v, err := stamp(ctx)
		if err != nil {
			_, err = fmt.Fprintf(w, "event: error\ndata: %q\n\n", err.Error())
		} else if v != last {
			last = v
			_, err = fmt.Fprintf(w, "event: version\ndata: %s\n\n", v)
		} else {
			return true
		}
		flusher.Flush()
		return err == nil
	}
	if !send() {
		return
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	beat := time.NewTicker(heartbeatEvery)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !send() {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
