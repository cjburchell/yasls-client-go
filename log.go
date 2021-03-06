package log

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cjburchell/tools-go/trace"
	"github.com/cjburchell/uatu-go/publishers"
	"github.com/pkg/errors"
)

// Level of the log
type Level struct {
	// Text representation of the log
	Text string
	// Severity value of the log
	Severity int
}

var (
	// DEBUG log level
	DEBUG = Level{Text: "Debug", Severity: 0}
	// INFO log level
	INFO = Level{Text: "Info", Severity: 1}
	// WARNING log level
	WARNING = Level{Text: "Warning", Severity: 2}
	// ERROR log level
	ERROR = Level{Text: "Error", Severity: 3}
	// FATAL log level
	FATAL = Level{Text: "Fatal", Severity: 4}
)

// ILog interface
type ILog interface {
	Warnf(format string, v ...interface{})
	Warn(v ...interface{})
	Error(err error, v ...interface{})
	Errorf(err error, format string, v ...interface{})
	Fatal(err error, v ...interface{})
	Fatalf(err error, format string, v ...interface{})
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	GetWriter(level Level) io.Writer
}

type logger struct {
	publishers []publishers.Publisher
	settings   Settings
	hostname   string
}

// Create the logger
func Create(settings Settings) ILog {
	var hostname, _ = os.Hostname()

	l := logger{
		settings: settings,
		hostname: hostname,
	}

	newPublishers := make([]publishers.Publisher, 0)
	if settings.UsePubSub {
		publisher, err := publishers.SetupPubSub(l.settings.PubSubSettings)
		if err != nil {
			log.Printf("Unable to create pub sub publisher %s", err.Error())
		} else {
			newPublishers = append(newPublishers, publisher)
		}
	}

	if settings.UseHTTP {
		publisher := publishers.SetupHTTP(l.settings.HTTPSettings)
		newPublishers = append(newPublishers, publisher)
	}

	l.publishers = newPublishers

	return l
}

// GetLogLevel gets the log level for input text
func GetLogLevel(levelText string) Level {
	var levels = []Level{DEBUG,
		INFO,
		WARNING,
		ERROR,
		FATAL,
	}

	for i := range levels {
		if levels[i].Text == levelText {
			return levels[i]
		}
	}

	return INFO
}

// Warnf Print a formatted warning level message
func (l logger) Warnf(format string, v ...interface{}) {
	l.printLog(fmt.Sprintf(format, v...), WARNING)
}

// Warn Print a warning message
func (l logger) Warn(v ...interface{}) {
	l.printLog(fmt.Sprint(v...), WARNING)
}

// Error Print a error level message
func (l logger) Error(err error, v ...interface{}) {
	l.printErrorLog(err, fmt.Sprint(v...), ERROR)
}

// Errorf Print a formatted error level message
func (l logger) Errorf(err error, format string, v ...interface{}) {
	l.printErrorLog(err, fmt.Sprintf(format, v...), ERROR)
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func (l logger) printErrorLog(err error, msg string, level Level) {
	if err == nil {
		l.printLog(msg, level)
		return
	}

	if msg == "" {
		msg = fmt.Sprintf("Error: %s\n", err.Error())
	} else {
		msg = fmt.Sprintf("%s\nError: %s\n", msg, err.Error())
	}

	if err, ok := err.(stackTracer); ok {
		msg += "Stack Trace -----------------------------------------------------------------------------------------\n"
		for _, f := range err.StackTrace() {
			msg += fmt.Sprintf("%+v\n", f)
		}
		msg += "-----------------------------------------------------------------------------------------------------"
	} else {
		msg += trace.GetStack(2)
	}

	l.printLog(msg, level)
}

// Fatal print fatal level message
func (l logger) Fatal(err error, v ...interface{}) {
	l.printErrorLog(err, fmt.Sprint(v...), FATAL)
	log.Panic(v...)
}

// Fatalf print formatted fatal level message
func (l logger) Fatalf(err error, format string, v ...interface{}) {
	l.printErrorLog(err, fmt.Sprintf(format, v...), FATAL)
	log.Panicf(format, v...)
}

// Debug print debug level message
func (l logger) Debug(v ...interface{}) {
	l.printLog(fmt.Sprint(v...), DEBUG)
}

// Debugf print formatted debug level  message
func (l logger) Debugf(format string, v ...interface{}) {
	l.printLog(fmt.Sprintf(format, v...), DEBUG)
}

// Print print info level message
func (l logger) Print(v ...interface{}) {
	l.printLog(fmt.Sprint(v...), INFO)
}

// Printf print info level message
func (l logger) Printf(format string, v ...interface{}) {
	l.printLog(fmt.Sprintf(format, v...), INFO)
}

// Message to be sent to centralized logger
type Message struct {
	Text        string `json:"text"`
	Level       Level  `json:"level"`
	ServiceName string `json:"serviceName"`
	Time        int64  `json:"time"`
	Hostname    string `json:"hostname"`
}

func (message Message) String() string {
	return fmt.Sprintf("[%s] %s %s - %s", message.Level.Text, time.Unix(message.Time/1000, 0).Format("2006-01-02 15:04:05 MST"), message.ServiceName, message.Text)
}

func (l logger) printLog(text string, level Level) {
	message := Message{
		Text:        text,
		Level:       level,
		ServiceName: l.settings.ServiceName,
		Time:        time.Now().UnixNano() / 1000000,
		Hostname:    l.hostname,
	}

	if level.Severity >= l.settings.MinLogLevel.Severity && l.settings.LogToConsole {
		if strings.HasSuffix(message.String(), "\n") {
			fmt.Print(message.String())
		} else {
			fmt.Println(message.String())
		}
	}

	if l.publishers == nil {
		return
	}

	messageBites, err := json.Marshal(message)
	if err != nil {
		fmt.Println("error:", err)
	}

	for _, publisher := range l.publishers {
		err = publisher.Publish(messageBites)
		if err != nil {
			fmt.Printf("Unable to send log to publisher (%s): %s", err.Error(), message.String())
		}
	}

}

func (l logger) GetWriter(level Level) io.Writer {
	return Writer{level, l}
}

// Writer for logs
type Writer struct {
	Level  Level
	logger logger
}

func (w Writer) Write(p []byte) (n int, err error) {
	w.logger.printLog(string(p), w.Level)
	return len(p), nil
}
