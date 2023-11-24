package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
)

// CustomNewRelicContextLogFormatter is a logrus.Formatter that will format logs for sending
// to New Relic. This is a custom implementation that includes sending all logrus.Entry.Fields,
// which isn't supported out of the box yet
//
// Based off of: https://github.com/newrelic/go-agent/blob/f1942e10f0819e2c854d5d7289eb0dc1c52a00af/v3/integrations/logcontext-v2/nrlogrus/formatter.go
type CustomNewRelicContextLogFormatter struct {
	app       *newrelic.Application
	formatter logrus.Formatter
}

func NewCustomNewRelicLogFormatter(app *newrelic.Application, formatter logrus.Formatter) CustomNewRelicContextLogFormatter {
	return CustomNewRelicContextLogFormatter{
		app:       app,
		formatter: formatter,
	}
}

func (f CustomNewRelicContextLogFormatter) Format(e *logrus.Entry) ([]byte, error) {
	message := e.Message
	if len(e.Data) > 0 {
		errorString := "<nil>"
		extraData := make(map[string]interface{})

		for k, v := range e.Data {
			if k == "error" {
				switch typed := v.(type) {
				case error:
					errorString = fmt.Sprintf("\"%s\"", typed.Error())
				default:
				}
			} else {
				extraData[k] = v
			}
		}

		extraDataJsonBytes, err := json.Marshal(extraData)
		if err == nil {
			message = fmt.Sprintf("message=\"%s\", error=%s, data=%s", message, errorString, string(extraDataJsonBytes))
		}
	}

	logData := newrelic.LogData{
		Severity: e.Level.String(),
		Message:  message,
	}

	logBytes, err := f.formatter.Format(e)
	if err != nil {
		return nil, err
	}
	logBytes = bytes.TrimRight(logBytes, "\n")
	b := bytes.NewBuffer(logBytes)

	ctx := e.Context
	var txn *newrelic.Transaction
	if ctx != nil {
		txn = newrelic.FromContext(ctx)
	}
	if txn != nil {
		txn.RecordLog(logData)
		err := newrelic.EnrichLog(b, newrelic.FromTxn(txn))
		if err != nil {
			return nil, err
		}
	} else {
		f.app.RecordLog(logData)
		err := newrelic.EnrichLog(b, newrelic.FromApp(f.app))
		if err != nil {
			return nil, err
		}
	}
	b.WriteString("\n")
	return b.Bytes(), nil
}
