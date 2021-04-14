package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/infrawatch/apputils/logging"
	"github.com/infrawatch/sg-core/pkg/application"
	"github.com/infrawatch/sg-core/pkg/config"
	"github.com/infrawatch/sg-core/pkg/data"
)

type configT struct {
	MetricOutput string
	EventsOutput string
}

type eventOutput struct {
	Index       string
	Type        string
	Publisher   string
	Severity    data.EventSeverity
	Labels      map[string]interface{}
	Annotations map[string]interface{}
}

type metricWrapper struct {
	last   time.Time
	metric data.Metric
}

// Print plugin suites for logging both internal buses to a file.
type Print struct {
	configuration configT
	logger        *logging.Logger
	eChan         chan data.Event
	mChan         chan data.Metric
	metrics       sync.Map
}

// New constructor
func New(logger *logging.Logger) application.Application {
	return &Print{
		configuration: configT{
			MetricOutput: "/dev/stdout",
			EventsOutput: "/dev/stdout",
		},
		logger: logger,
		eChan:  make(chan data.Event, 5),
		mChan:  make(chan data.Metric, 5),
	}
}

// ReceiveEvent ...
func (p *Print) ReceiveEvent(e data.Event) {
	p.eChan <- e
}

// ReceiveMetric ...
func (p *Print) ReceiveMetric(name string, t float64, mType data.MetricType, interval time.Duration, value float64, labelKeys []string, labelVals []string) {
	metric := data.Metric{
		Name:      name,
		Time:      t,
		Type:      mType,
		Interval:  interval,
		Value:     value,
		LabelKeys: labelKeys,
		LabelVals: labelVals,
	}
	p.mChan <- metric
}

// Run run scrape endpoint
func (p *Print) Run(ctx context.Context, done chan bool) {

	metrF, err := os.OpenFile(p.configuration.MetricOutput, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		p.logger.Metadata(logging.Metadata{"plugin": "print", "error": err})
		p.logger.Error("failed to open metrics data output file")
	} else {
		defer metrF.Close()
	}

	evtsF, errr := os.OpenFile(p.configuration.EventsOutput, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		p.logger.Metadata(logging.Metadata{"plugin": "print", "error": err})
		p.logger.Error("failed to open events data output file")
	} else {
		defer evtsF.Close()
	}

	if err == nil && errr == nil {
		p.logger.Metadata(logging.Metadata{"plugin": "print", "events": p.configuration.EventsOutput, "metrics": p.configuration.MetricOutput})
		p.logger.Info("writing processed data to files.")

		for {
			select {
			case <-ctx.Done():
				goto done
			case event := <-p.eChan:
				eo := eventOutput{
					Index:       event.Index,
					Type:        event.Type.String(),
					Publisher:   event.Publisher,
					Severity:    event.Severity,
					Labels:      event.Labels,
					Annotations: event.Annotations,
				}
				encoded, err := json.MarshalIndent(eo, "", "  ")
				if err != nil {
					p.logger.Metadata(logging.Metadata{"plugin": "print", "data": event})
					p.logger.Warn("failed to marshal event data")
				}
				evtsF.WriteString(fmt.Sprintf("Processed event:\n%s\n", string(encoded)))
			case metrics := <-p.mChan:
				encoded, err := json.MarshalIndent(metrics, "", "  ")
				if err != nil {
					p.logger.Metadata(logging.Metadata{"plugin": "print", "data": metrics})
					p.logger.Warn("failed to marshal metric data")
				}
				metrF.WriteString(fmt.Sprintf("Processed metric:\n%s\n", string(encoded)))
			}
		}
	}
done:
	p.logger.Metadata(logging.Metadata{"plugin": "print"})
	p.logger.Info("exited")
}

// Config implements application.Application
func (p *Print) Config(c []byte) error {
	err := config.ParseConfig(bytes.NewReader(c), &p.configuration)
	if err != nil {
		return err
	}
	return nil
}
