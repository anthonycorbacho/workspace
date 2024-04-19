// Package log provides function for managing structured logs.
// You can create a new logger by calling the New function
//
//		// Create a production logger with Info level.
//		logger, _ := log.New()
//		defer logger.Close()
//
//		// Create a production logger with Debug level
//		logger, _ := log.New(log.WithLevel(log.DebugLevel))
//		defer logger.Close()
//
package log
