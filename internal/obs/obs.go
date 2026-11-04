// Package obs is the small observability surface a single-binary service needs:
// JSON logs a machine can parse, and counters a scraper can read.
package obs

import (
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	logger atomic.Pointer[slog.Logger]
	once   sync.Once
)

// Init installs the process logger. jsonLines writes one JSON object per line;
// otherwise a text handler is used for humans.
func Init(w io.Writer, jsonLines bool) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler
	if jsonLines {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	logger.Store(slog.New(h))
}

// Log returns the process logger, installing a discard logger if Init was
// never called.
func Log() *slog.Logger {
	once.Do(func() {
		if logger.Load() == nil {
			logger.Store(slog.New(slog.NewTextHandler(io.Discard, nil)))
		}
	})
	return logger.Load()
}

// With returns a component-scoped logger.
func With(component string) *slog.Logger { return Log().With("component", component) }

// ---- metrics ----

var (
	counters sync.Map // name -> *int64
	gauges   sync.Map // name -> func() int64
)

func counter(name string) *int64 {
	v, _ := counters.LoadOrStore(name, new(int64))
	return v.(*int64)
}

// Inc adds one to a counter.
func Inc(name string) { atomic.AddInt64(counter(name), 1) }

// Add adds n to a counter.
func Add(name string, n int64) { atomic.AddInt64(counter(name), n) }

// Set creates or updates a gauge backed by fn.
func Set(name string, fn func() int64) { gauges.Store(name, fn) }

// Render writes every metric in Prometheus text exposition format.
func Render() string {
	var b strings.Builder
	keys := make([]string, 0, 16)
	counters.Range(func(k, _ any) bool { keys = append(keys, k.(string)); return true })
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := counters.Load(k)
		b.WriteString("# TYPE " + k + " counter\n")
		b.WriteString(k + " " + itoa(atomic.LoadInt64(v.(*int64))) + "\n")
	}

	gkeys := make([]string, 0, 8)
	gauges.Range(func(k, _ any) bool { gkeys = append(gkeys, k.(string)); return true })
	sort.Strings(gkeys)
	for _, k := range gkeys {
		fn, _ := gauges.Load(k)
		b.WriteString("# TYPE " + k + " gauge\n")
		b.WriteString(k + " " + itoa(fn.(func() int64)()) + "\n")
	}
	return b.String()
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Reset clears every metric. Tests use it.
func Reset() {
	counters.Range(func(k, _ any) bool { counters.Delete(k); return true })
	gauges.Range(func(k, _ any) bool { gauges.Delete(k); return true })
}
