package logging

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

type Level int32

const (
	LevelError Level = iota
	LevelInfo
	LevelDebug
)

const HeaderRequestID = "X-Request-ID"

type ctxKey struct{}

var currentLevel atomic.Int32

func init() {
	currentLevel.Store(int32(LevelInfo))
}

// Configure sets the minimum log level: error | info | debug (default info).
func Configure(level string) {
	currentLevel.Store(int32(ParseLevel(level)))
}

func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "dbg", "trace":
		return LevelDebug
	case "error", "err":
		return LevelError
	case "warn", "warning":
		return LevelInfo // no separate warn; treat as info floor
	default:
		return LevelInfo
	}
}

func CurrentLevel() Level {
	return Level(currentLevel.Load())
}

// Enabled reports whether logs at level l should be emitted for the current config.
// Levels are ordered: error < info < debug (debug includes everything).
func Enabled(l Level) bool {
	return l <= CurrentLevel()
}

func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// ResolveRequestID uses client header if valid, otherwise generates a new id.
func ResolveRequestID(header string) string {
	if id := sanitizeRequestID(header); id != "" {
		return id
	}
	return NewID()
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", atomic.AddInt64(&fallbackSeq, 1))
	}
	// UUID v4-ish
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var fallbackSeq int64

func sanitizeRequestID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 {
		return ""
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if !ok {
			return ""
		}
	}
	return s
}

func Errorf(ctx context.Context, msg string, kv ...any) { logf(ctx, LevelError, msg, kv...) }
func Infof(ctx context.Context, msg string, kv ...any)  { logf(ctx, LevelInfo, msg, kv...) }
func Debugf(ctx context.Context, msg string, kv ...any) { logf(ctx, LevelDebug, msg, kv...) }

// Startup logs without request context.
func Info(msg string, kv ...any)  { logf(context.Background(), LevelInfo, msg, kv...) }
func Error(msg string, kv ...any) { logf(context.Background(), LevelError, msg, kv...) }
func Debug(msg string, kv ...any) { logf(context.Background(), LevelDebug, msg, kv...) }

func logf(ctx context.Context, level Level, msg string, kv ...any) {
	if !Enabled(level) {
		return
	}
	reqID := RequestID(ctx)
	if reqID == "" {
		reqID = "-"
	}
	log.Printf("req_id=%s level=%s msg=%s%s", reqID, levelName(level), msg, formatKV(kv))
}

func levelName(l Level) string {
	switch l {
	case LevelError:
		return "error"
	case LevelDebug:
		return "debug"
	default:
		return "info"
	}
}

func formatKV(kv []any) string {
	if len(kv) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		key := fmt.Sprint(kv[i])
		val := ""
		if i+1 < len(kv) {
			val = fmt.Sprint(kv[i+1])
		}
		val = strings.ReplaceAll(val, " ", "_")
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(val)
	}
	return b.String()
}
