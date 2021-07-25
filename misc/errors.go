package misc

import "DistributedMandelbrot/log"

const (
	Fatal Severity = iota
	Error
	Warning
	Info
	Debug
)

type Severity int

func (s Severity) String() string {
	return []string{
		"Fatal", "Error", "Warning", "Info", "Debug",
	}[s]
}

func CheckError(err error, logger log.Logger, severity Severity) {
	if err != nil {
		switch severity {
		case Fatal:
			logger.Fatal(err.Error())
		case Error:
			logger.Error(err.Error())
		case Warning:
			logger.Warning(err.Error())
		case Info:
			logger.Info(err.Error())
		case Debug:
			logger.Debug(err.Error())
		default:
			logger.Fatal(err.Error())
		}
	}
}
