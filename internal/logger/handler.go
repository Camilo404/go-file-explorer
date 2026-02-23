package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	purple = "\033[35m"
	cyan   = "\033[36m"
	gray   = "\033[37m"
	white  = "\033[97m"
)

type PrettyHandler struct {
	opts  slog.HandlerOptions
	w     io.Writer
	mu    *sync.Mutex
	attrs []slog.Attr
	group string
}

func NewPrettyHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &PrettyHandler{
		opts:  *opts,
		w:     w,
		mu:    &sync.Mutex{},
		attrs: []slog.Attr{},
	}
}

func (h *PrettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Timestamp
	timeStr := r.Time.Format("15:04:05.000")
	fmt.Fprintf(h.w, "%s%s%s ", gray, timeStr, reset)

	// Level
	level := r.Level.String()
	var levelColor string
	switch r.Level {
	case slog.LevelDebug:
		levelColor = purple
	case slog.LevelInfo:
		levelColor = green
	case slog.LevelWarn:
		levelColor = yellow
	case slog.LevelError:
		levelColor = red
	default:
		levelColor = white
	}
	
	// Pad level to 5 chars (INFO , ERROR, DEBUG, WARN )
	fmt.Fprintf(h.w, "%s%-5s%s ", levelColor, level, reset)

	// Message
	fmt.Fprintf(h.w, "%s%s%s", white, r.Message, reset)

	// Stored attributes (from WithAttrs)
	for _, a := range h.attrs {
		h.printAttr(a)
	}

	// Record attributes
	r.Attrs(func(a slog.Attr) bool {
		h.printAttr(a)
		return true
	})

	fmt.Fprintln(h.w)
	return nil
}

func (h *PrettyHandler) printAttr(a slog.Attr) {
	key := a.Key
	if h.group != "" {
		key = h.group + "." + key
	}
	
	val := a.Value.Any()
	// Format time if applicable
	if t, ok := val.(time.Time); ok {
		val = t.Format(time.RFC3339)
	}
	
	fmt.Fprintf(h.w, " %s%s%s=%v", cyan, key, reset, val)
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	
	return &PrettyHandler{
		opts:  h.opts,
		w:     h.w,
		mu:    h.mu, // Share mutex for writing to same output
		attrs: newAttrs,
		group: h.group,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	
	return &PrettyHandler{
		opts:  h.opts,
		w:     h.w,
		mu:    h.mu,
		attrs: h.attrs,
		group: newGroup,
	}
}
