package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// StreamingTimeout creates middleware for file transfer routes that does NOT
// buffer responses in memory (unlike http.TimeoutHandler). It enforces:
//   - maxDuration: absolute maximum time for the entire transfer.
//   - idleTimeout: maximum time between consecutive writes; if no data is
//     written for this period the connection is killed (stale transfer).
//
// This middleware preserves http.Flusher so http.ServeContent can stream
// partial responses (Range / 206) directly to the client.
func StreamingTimeout(maxDuration, idleTimeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), maxDuration)
			defer cancel()

			// Set connection-level deadlines so blocked Writes are unblocked
			// when the deadline expires (http.ResponseController, Go 1.20+).
			rc := http.NewResponseController(w)
			deadline := time.Now().Add(maxDuration)
			_ = rc.SetWriteDeadline(deadline)
			_ = rc.SetReadDeadline(deadline)

			sw := &streamingWriter{
				ResponseWriter: w,
				rc:             rc,
				idleTimeout:    idleTimeout,
				cancel:         cancel,
			}
			sw.resetIdle()

			next.ServeHTTP(sw, r.WithContext(ctx))

			sw.mu.Lock()
			if sw.idleTimer != nil {
				sw.idleTimer.Stop()
			}
			sw.mu.Unlock()
		})
	}
}

// streamingWriter wraps http.ResponseWriter with an inactivity timer.
// Every Write resets the idle countdown. If no writes happen within
// idleTimeout the context is cancelled and the connection deadline is
// shortened so in-flight I/O fails fast.
type streamingWriter struct {
	http.ResponseWriter
	rc          *http.ResponseController
	idleTimeout time.Duration
	cancel      context.CancelFunc
	mu          sync.Mutex
	idleTimer   *time.Timer
}

func (sw *streamingWriter) resetIdle() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.idleTimer != nil {
		sw.idleTimer.Stop()
	}

	sw.idleTimer = time.AfterFunc(sw.idleTimeout, func() {
		// Shorten the connection deadline so blocked writes fail immediately.
		_ = sw.rc.SetWriteDeadline(time.Now())
		sw.cancel()
	})
}

// Write implements io.Writer, resetting the idle timer on each successful write.
func (sw *streamingWriter) Write(b []byte) (int, error) {
	sw.resetIdle()
	return sw.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController and middleware reach the real writer.
func (sw *streamingWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

// Flush implements http.Flusher so streaming responses (SSE, ServeContent)
// flush data to the client immediately instead of accumulating in buffers.
func (sw *streamingWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
