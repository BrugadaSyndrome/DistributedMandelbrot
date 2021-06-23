package log

import (
	"fmt"
	"log"
	"os"
)

type ansiEscapeCode int

const (
	// Reset previous codes
	reset ansiEscapeCode = 0

	// Display attributes
	normal           = 22
	bold             = 1
	faint            = 2
	italic           = 3
	noItalic         = 23
	underline        = 4
	doubleUnderline  = 21
	noUnderline      = 24
	invert           = 7
	noInvert         = 27
	strike           = 9
	noStrike         = 29
	fontDefault      = 10
	framed           = 51
	encircled        = 52
	noFrameEncircled = 54

	// Foreground colors
	fgDefault      = 39
	fgBlack        = 30
	fgRed          = 31
	fgGreen        = 32
	fgYellow       = 33
	fgBlue         = 34
	fgPurple       = 35
	fgCyan         = 36
	fgWhite        = 37
	fgBrightBlack  = 90
	fgBrightRed    = 91
	fgBrightGreen  = 92
	fgBrightYellow = 93
	fgBrightBlue   = 94
	fgBrightPurple = 95
	fgBrightCyan   = 96
	fgBrightWhite  = 97

	// Background colors
	bgDefault      = 49
	bgBlack        = 40
	bgRed          = 41
	bgGreen        = 42
	bgYellow       = 43
	bgBlue         = 44
	bgPurple       = 45
	bgCyan         = 46
	bgWhite        = 47
	bgBrightBlack  = 100
	bgBrightRed    = 101
	bgBrightGreen  = 102
	bgBrightYellow = 103
	bgBrightBlue   = 104
	bgBrightPurple = 105
	bgBrightCyan   = 106
	bgBrightWhite  = 107
)

type verbosity int

const (
	Minimal verbosity = iota
	Normal
	All
)

func (v verbosity) String() string {
	return []string{
		"Minimal", "Normal", "All",
	}[v]
}

type Logger struct {
	logFile   *os.File
	name      string
	verbosity verbosity

	Logger *log.Logger
}

func NewLogger(flags int, name string, verbosity verbosity, logFile *os.File) Logger {
	logger := Logger{
		name:      fmt.Sprintf("[%s] ", name),
		verbosity: verbosity,
		Logger:    log.New(os.Stdout, "", flags),

		logFile: logFile,
	}
	return logger
}

func (l *Logger) Fatal(message string) {
	if l.logFile != nil {
		l.Logger.SetOutput(l.logFile)
		l.Logger.SetPrefix(fmt.Sprintf("%sFATAL: %s", l.name, message))
		l.Logger.Print(message)
	}

	l.Logger.SetOutput(os.Stderr)
	l.Logger.SetPrefix(l.name + ansiEscapeEncode("FATAL: ", fgBrightRed, bgDefault, framed))
	l.Logger.Fatalf(ansiEscapeEncode(message, fgBrightRed, bgDefault, framed))
}

func (l *Logger) Fatalf(format string, values ...interface{}) {
	l.Fatal(fmt.Sprintf(format, values...))
}

func (l *Logger) Error(message string) {
	if l.verbosity < Minimal {
		return
	}

	if l.logFile != nil {
		l.Logger.SetOutput(l.logFile)
		l.Logger.SetPrefix(fmt.Sprintf("%sERROR: ", l.name))
		l.Logger.Print(message)
	}

	l.Logger.SetOutput(os.Stderr)
	l.Logger.SetPrefix(l.name + ansiEscapeEncode("ERROR: ", fgRed, bgDefault, normal))
	l.Logger.Print(ansiEscapeEncode(message, fgRed, bgDefault, normal))
}

func (l *Logger) Errorf(format string, values ...interface{}) {
	l.Error(fmt.Sprintf(format, values...))
}

func (l *Logger) Warning(message string) {
	if l.verbosity < Normal {
		return
	}

	if l.logFile != nil {
		l.Logger.SetOutput(l.logFile)
		l.Logger.SetPrefix(fmt.Sprintf("%sWARNING: ", l.name))
		l.Logger.Print(message)
	}

	l.Logger.SetOutput(os.Stdout)
	l.Logger.SetPrefix(l.name + ansiEscapeEncode("WARNING: ", fgYellow, bgDefault, normal))
	l.Logger.Print(ansiEscapeEncode(message, fgYellow, bgDefault, normal))
}

func (l *Logger) Warningf(format string, values ...interface{}) {
	l.Warning(fmt.Sprintf(format, values...))
}

func (l *Logger) Info(message string) {
	if l.verbosity < Normal {
		return
	}

	if l.logFile != nil {
		l.Logger.SetOutput(l.logFile)
		l.Logger.SetPrefix(fmt.Sprintf("%sINFO: ", l.name))
		l.Logger.Print(message)
	}

	l.Logger.SetOutput(os.Stdout)
	l.Logger.SetPrefix(l.name + ansiEscapeEncode("INFO: ", fgBlue, bgDefault, normal))
	l.Logger.Print(ansiEscapeEncode(message, fgBlue, bgDefault, normal))
}

func (l *Logger) Infof(format string, values ...interface{}) {
	l.Info(fmt.Sprintf(format, values...))
}

func (l *Logger) Debug(message string) {
	if l.verbosity < All {
		return
	}

	if l.logFile != nil {
		l.Logger.SetOutput(l.logFile)
		l.Logger.SetPrefix(fmt.Sprintf("%sDEBUG: ", l.name))
		l.Logger.Print(message)
	}

	l.Logger.SetOutput(os.Stdout)
	l.Logger.SetPrefix(l.name + ansiEscapeEncode("DEBUG: ", fgPurple, bgDefault, normal))
	l.Logger.Print(ansiEscapeEncode(message, fgPurple, bgDefault, normal))
}

func (l *Logger) Debugf(format string, values ...interface{}) {
	l.Debug(fmt.Sprintf(format, values...))
}

func ansiEscapeEncode(message string, fg ansiEscapeCode, bg ansiEscapeCode, display ansiEscapeCode) string {
	// Ansi escape codes do not work in windows terminal
	/*
		if runtime.GOOS == "windows" {
			return message
		}
	*/
	return fmt.Sprintf("\033[%d;%d;%dm%s\033[0m", fg, bg, display, message)
}
