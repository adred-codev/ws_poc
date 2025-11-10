package monitoring

import (
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/adred-codev/ws_poc/internal/single/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Level  types.LogLevel  // Minimum log level
	Format types.LogFormat // Output format
}

// NewLogger creates a structured logger configured for Loki integration
//
// Features:
//   - Structured JSON output (Loki-compatible)
//   - Stack traces on errors
//   - Contextual fields for filtering
//   - Timestamp in RFC3339 format
//   - Caller information for debugging
//
// Example:
//
//	logger := NewLogger(LoggerConfig{
//	    Level: LogLevelInfo,
//	    Format: LogFormatJSON,
//	})
//	logger.Info().
//	    Str("component", "server").
//	    Int("connections", 100).
//	    Msg("Server started")
func NewLogger(config LoggerConfig) zerolog.Logger {
	var output io.Writer = os.Stdout

	// Set log level
	var level zerolog.Level
	switch config.Level {
	case types.LogLevelDebug:
		level = zerolog.DebugLevel
	case types.LogLevelInfo:
		level = zerolog.InfoLevel
	case types.LogLevelWarn:
		level = zerolog.WarnLevel
	case types.LogLevelError:
		level = zerolog.ErrorLevel
	case types.LogLevelFatal:
		level = zerolog.FatalLevel
	default:
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Set format
	if config.Format == types.LogFormatPretty {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Create logger with timestamp and caller info
	logger := zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Str("service", "ws-server").
		Logger()

	return logger
}

// LogError logs an error with full context and optional stack trace
//
// Parameters:
//   - logger: The zerolog logger
//   - err: The error to log
//   - msg: Human-readable message
//   - fields: Additional context fields (key-value pairs)
//
// Example:
//
//	LogError(logger, err, "Failed to broadcast", map[string]any{
//	    "client_id": client.id,
//	    "message_size": len(data),
//	})
func LogError(logger zerolog.Logger, err error, msg string, fields map[string]any) {
	event := logger.Error().Err(err)

	// Add all context fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// LogErrorWithStack logs an error with stack trace
//
// Use this for unexpected errors, panics recovered, or critical failures
// where you need to understand the full call stack.
//
// Example:
//
//	defer func() {
//	    if r := recover(); r != nil {
//	        LogErrorWithStack(logger, fmt.Errorf("panic: %v", r), "Worker panic recovered", nil)
//	    }
//	}()
func LogErrorWithStack(logger zerolog.Logger, err error, msg string, fields map[string]any) {
	stack := string(debug.Stack())

	event := logger.Error().Err(err).Str("stack_trace", stack)

	// Add all context fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// LogPanic logs a recovered panic with full stack trace
//
// Use in defer recover() blocks to log panics before re-panicking or gracefully handling.
//
// Example:
//
//	defer func() {
//	    if r := recover(); r != nil {
//	        LogPanic(logger, r, "Worker goroutine panic", map[string]any{
//	            "worker_id": id,
//	        })
//	    }
//	}()
func LogPanic(logger zerolog.Logger, panicValue any, msg string, fields map[string]any) {
	stack := string(debug.Stack())

	event := logger.Fatal().
		Interface("panic_value", panicValue).
		Str("stack_trace", stack)

	// Add all context fields
	for k, v := range fields {
		event = event.Interface(k, v)
	}

	event.Msg(msg)
}

// InitGlobalLogger initializes the global logger
// This should be called once at application startup
func InitGlobalLogger(config LoggerConfig) {
	logger := NewLogger(config)
	log.Logger = logger
}
