package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string level to Level
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG":
		return LevelDebug
	case "info", "INFO":
		return LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return LevelWarn
	case "error", "ERROR":
		return LevelError
	case "fatal", "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	Service   string         `json:"service,omitempty"`
	File      string         `json:"file,omitempty"`
	Line      int            `json:"line,omitempty"`
	Duration  float64        `json:"duration_ms,omitempty"`
	Error     string         `json:"error,omitempty"`
	Stack     string         `json:"stack,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger is a structured JSON logger
type Logger struct {
	mu      sync.Mutex
	level   Level
	output  io.Writer
	service string
	fields  map[string]any
}

// Config for logger
type Config struct {
	Level   Level
	Output  io.Writer
	Service string
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger
func Init(cfg Config) {
	once.Do(func() {
		if cfg.Output == nil {
			cfg.Output = os.Stdout
		}
		if cfg.Service == "" {
			cfg.Service = "backend"
		}
		defaultLogger = &Logger{
			level:   cfg.Level,
			output:  cfg.Output,
			service: cfg.Service,
			fields:  make(map[string]any),
		}
	})
}

// Default returns the default logger
func Default() *Logger {
	if defaultLogger == nil {
		Init(Config{Level: LevelInfo, Output: os.Stdout, Service: "backend"})
	}
	return defaultLogger
}

// New creates a new logger instance
func New(cfg Config) *Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	return &Logger{
		level:   cfg.Level,
		output:  cfg.Output,
		service: cfg.Service,
		fields:  make(map[string]any),
	}
}

// WithField returns a new logger with an additional field
func (l *Logger) WithField(key string, value any) *Logger {
	newLogger := &Logger{
		level:   l.level,
		output:  l.output,
		service: l.service,
		fields:  make(map[string]any),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields returns a new logger with additional fields
func (l *Logger) WithFields(fields map[string]any) *Logger {
	newLogger := &Logger{
		level:   l.level,
		output:  l.output,
		service: l.service,
		fields:  make(map[string]any),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

// WithContext extracts request_id and user_id from context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	newLogger := l.WithFields(nil)
	if reqID := ctx.Value("request_id"); reqID != nil {
		newLogger.fields["request_id"] = reqID
	}
	if userID := ctx.Value("user_id"); userID != nil {
		newLogger.fields["user_id"] = fmt.Sprintf("%v", userID)
	}
	return newLogger
}

// WithError adds error information
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return l.WithField("error", err.Error())
}

// WithDuration adds duration in milliseconds
func (l *Logger) WithDuration(d time.Duration) *Logger {
	return l.WithField("duration_ms", float64(d.Microseconds())/1000.0)
}

func (l *Logger) log(level Level, msg string, args ...any) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   fmt.Sprintf(msg, args...),
		Service:   l.service,
		Fields:    l.fields,
	}

	// Extract special fields
	if reqID, ok := l.fields["request_id"].(string); ok {
		entry.RequestID = reqID
		delete(entry.Fields, "request_id")
	}
	if userID, ok := l.fields["user_id"].(string); ok {
		entry.UserID = userID
		delete(entry.Fields, "user_id")
	}
	if errStr, ok := l.fields["error"].(string); ok {
		entry.Error = errStr
		delete(entry.Fields, "error")
	}
	if duration, ok := l.fields["duration_ms"].(float64); ok {
		entry.Duration = duration
		delete(entry.Fields, "duration_ms")
	}

	// Add caller info for error and fatal
	if level >= LevelError {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			entry.File = file
			entry.Line = line
		}
	}

	// Empty fields should be omitted
	if len(entry.Fields) == 0 {
		entry.Fields = nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.output, `{"level":"ERROR","message":"failed to marshal log entry: %s"}`+"\n", err)
		return
	}
	l.output.Write(append(data, '\n'))
}

// Log methods
func (l *Logger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args...) }
func (l *Logger) Info(msg string, args ...any)  { l.log(LevelInfo, msg, args...) }
func (l *Logger) Warn(msg string, args ...any)  { l.log(LevelWarn, msg, args...) }
func (l *Logger) Error(msg string, args ...any) { l.log(LevelError, msg, args...) }
func (l *Logger) Fatal(msg string, args ...any) {
	l.log(LevelFatal, msg, args...)
	os.Exit(1)
}

// Package-level functions using default logger
func Debug(msg string, args ...any) { Default().Debug(msg, args...) }
func Info(msg string, args ...any)  { Default().Info(msg, args...) }
func Warn(msg string, args ...any)  { Default().Warn(msg, args...) }
func Error(msg string, args ...any) { Default().Error(msg, args...) }
func Fatal(msg string, args ...any) { Default().Fatal(msg, args...) }

func WithField(key string, value any) *Logger  { return Default().WithField(key, value) }
func WithFields(fields map[string]any) *Logger { return Default().WithFields(fields) }
func WithContext(ctx context.Context) *Logger  { return Default().WithContext(ctx) }
func WithError(err error) *Logger              { return Default().WithError(err) }
func WithDuration(d time.Duration) *Logger     { return Default().WithDuration(d) }
