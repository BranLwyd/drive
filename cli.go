package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

var (
	isQuiet, isVerbose bool
)

func Setup(quiet, verbose bool) error {
	if quiet && verbose {
		return errors.New("cannot be both quiet and verbose")
	}
	isQuiet, isVerbose = quiet, verbose
	return nil
}

func Info(format string, args ...interface{}) {
	if isQuiet {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func Verbose(format string, args ...interface{}) {
	if !isVerbose {
		return
	}
	fmt.Printf(format+"\n", args...)
}

func Warning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func Die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func DieWithUsage(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n\n", args...)
	flag.Usage()
	os.Exit(1)
}
