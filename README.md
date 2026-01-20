# Flexible and Extensible Go Logger

A flexible and extensible logger for Go applications.

This logger provides a simple and consistent logging interface with support for various logging backends. It is designed to be easy to use and to integrate with other libraries and services.

## Installation

To install the logger package, use `go get`:

```bash
go get github.com/zauberhaus/logger
```

## Usage

To use the logger, you first need to get a logger instance. You can use the global logger or get it from a context.

```go
package main

import (
	"context"
	"github.com/zauberhaus/logger"
)

func main() {
	ctx := context.Background()

	// Get default logger
	log = logger.GetLogger(ctx)

	// Create a new logger instance
	log := zap.NewLogger()

	// Set the global logger
	logger.SetLogger(log)

	// Get the global logger
	log = logger.GetLogger(ctx)

	// Add logger to context
	ctx = logger.AddLogger(ctx, l)

	// Get logger from context 
	log = logger.GetLogger(ctx)

	// Log a message
	log.Info("This is an info message")

	// Log a formatted message
	log.Debugf("This is a debug message with a value: %d", 42)

	// Set the log level
	logger.SetLevel(logger.DebugLevel)

	// Check the log level
	if log.HasLevel(logger.DebugLevel) {
		log.Debug("Debug logging is enabled")
	}
}
```

## Logger Implementations

This logger supports several implementations allowing you to choose the one that best fits your needs.

### Zap

The default logger implementation is based on [Uber's Zap](https://github.com/uber-go/zap), a fast, structured, and leveled logging library.

### Memory

The Memory logger is based on the zap logger and stores log messages in memory. This is useful for testing and debugging, as it allows you to inspect the logged messages after the fact.

### Filtered

The Filtered logger is a wrapper around the zap logger that filters log messages based on a set of rules. This can be used to suppress log messages that you are not interested in.

## Adapter

### Redis

The `redis` package provides a adapter to make the a logger compatible with Redis clients like `go-redis`. It provides an implementation that can be passed to a Redis client to handle logging of commands and connection events, using the application's logger.

### Temporal

The `temporal` package provides an adapter to use the logger with the [Temporal](https://temporal.io/) SDK. It implements the `log.Logger` interface from `go.temporal.io/sdk/log`, allowing the application's logger to be used seamlessly within Temporal workflows and activities.

## Backends

### Sentry

The Sentry backend sends log messages to [Sentry](https://sentry.io/), an error tracking and monitoring platform.


## Testing

To run the tests, use the `tests.sh` script:

```bash
./tests.sh
```

This script will run all the tests in the project, generate a coverage report, and format the output.
