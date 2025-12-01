package app

import (
	"fmt"
)

// Log prints a formatted log message with a simple package prefix.
func Log(pkg string, format string, args ...interface{}) {
	prefix := ""
	if pkg != "" {
		prefix = "[" + pkg + "] "
	}
	fmt.Printf(prefix+format+"\n", args...)
}
