package utils

import (
	"github.com/onrik/logrus/filename"
	"github.com/sirupsen/logrus"
)

func SetLog() {
	Formatter := new(logrus.TextFormatter)
	Formatter.TimestampFormat = "Jan _2 15:04:05.000000000"
	Formatter.FullTimestamp = true
	Formatter.ForceColors = true
	logrus.AddHook(filename.NewHook()) // Print filename + line at every log
	logrus.SetFormatter(Formatter)
}
