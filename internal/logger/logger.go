package logger

import (
	"go.uber.org/zap"
)

var err error

type Logger struct {
	*zap.Logger
}

func (l *Logger) init() error {
	l.Logger, err = zap.NewDevelopment()

	return err
}

// NewZapLogger takes the package name as an arg and returns a Logger
func NewZapLogger(pkg string) *Logger {
	Log := &Logger{}
	err = Log.init()
	if err != nil {
		panic(err)
	}

	Log.Logger = Log.Logger.With(
		zap.String("package", pkg),
	)

	return Log
}
