package log

import "log"

type Loggers struct {
	Csn *log.Logger
	Pollard *log.Logger
}

func (l *Loggers) SetLoggers(logger *log.Logger) {
	l.Csn = logger
	l.Pollard = logger
}

func UseLoggerForAll (logger *log.Logger) (Loggers) {
	var loggers Loggers
	loggers.SetLoggers(logger)
	return loggers
}
