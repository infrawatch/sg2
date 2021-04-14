package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/infrawatch/apputils/logging"
	"github.com/infrawatch/sg-core/pkg/application"
	"github.com/infrawatch/sg-core/pkg/config"
	"github.com/infrawatch/sg-core/pkg/data"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/errgo.v2/fmt/errors"
)

var typeToPromType map[data.MetricType]prometheus.ValueType = map[data.MetricType]prometheus.ValueType{
	data.COUNTER: prometheus.CounterValue,
	data.GAUGE:   prometheus.GaugeValue,
	data.UNTYPED: prometheus.UntypedValue,
}

type configT struct {
	Host               string
	Port               int  `validate:"required"`
	WithTimestamp      bool `yaml:"withTimeStamp"`
	ExpirationMultiple int  `yaml:"expirationMultiple"` // multiple of metric interval at which that metric should be expired (seconds)
	ServeCache         bool `yaml:"serveCache"`         // serve cache at /content
}

// used to expire stale metrics
type metricExpiry struct {
	sync.RWMutex
	lastArrival time.Time
	delete      func() bool
}

func (me *metricExpiry) keepAlive() {
	me.Lock()
	defer me.Unlock()
	me.lastArrival = time.Now()
}

func (me *metricExpiry) Expired(interval time.Duration) bool {
	me.RLock()
	defer me.RUnlock()
	return (time.Since(me.lastArrival) >= interval)
}

func (me *metricExpiry) Delete() bool {
	me.Lock()
	defer me.Unlock()
	return me.delete()
}

type collectorExpiry struct {
	sync.RWMutex
	collector *PromCollector
	delete    func() bool
}

func (ce *collectorExpiry) Expired(interval time.Duration) bool {
	return (syncMapLen(&ce.collector.mProc) == 0)
}

func (ce *collectorExpiry) Delete() bool {
	ce.Lock()
	defer ce.Unlock()
	return ce.delete()
}

type logWrapper struct {
	l      *logging.Logger
	plugin string
}

func (lw *logWrapper) Error(msg string, err error) {
	lw.l.Metadata(logging.Metadata{"plugin": lw.plugin, "error": err})
	lw.l.Error(msg)
}

func (lw *logWrapper) Warn(msg string) {
	lw.l.Metadata(logging.Metadata{"plugin": lw.plugin})
	lw.l.Warn(msg)
}

func (lw *logWrapper) Infof(format string, a ...interface{}) {
	lw.l.Metadata(logging.Metadata{"plugin": lw.plugin})
	lw.l.Info(fmt.Sprintf(format, a...))
}

func (lw *logWrapper) Debugf(format string, a ...interface{}) {
	lw.l.Metadata(logging.Metadata{"plugin": lw.plugin})
	lw.l.Debug(fmt.Sprintf(format, a...))
}

// container object for all metric related processes
type metricProcess struct {
	description *prometheus.Desc
	expiry      *metricExpiry
	metric      *data.Metric
	scrapped    bool
}

// PromCollector implements prometheus.Collector for incoming metrics. Metrics
// with differing label dimensions must create separate PromCollectors.
type PromCollector struct {
	logger            *logWrapper
	mProc             sync.Map
	dimensions        int
	withtimestamp     bool
	cacheindexbuilder strings.Builder
}

// NewPromCollector PromCollector constructor
func NewPromCollector(l *logWrapper, dimensions int, withtimestamp bool) *PromCollector {
	return &PromCollector{
		logger:        l,
		dimensions:    dimensions,
		withtimestamp: withtimestamp,
	}
}

// Describe implements prometheus.Collector
func (pc *PromCollector) Describe(ch chan<- *prometheus.Desc) {
	pc.mProc.Range(func(mName interface{}, itf interface{}) bool {
		ch <- itf.(*metricProcess).description
		return true
	})
}

// Collect implements prometheus.Collector
func (pc *PromCollector) Collect(ch chan<- prometheus.Metric) {
	// fmt.Printf("\nScrapping collector of size %d with %d metrics:\n", pc.dimensions, syncMapLen(&pc.mProc))
	pc.mProc.Range(func(mName interface{}, itf interface{}) bool {
		// fmt.Println(mName)
		mProc := itf.(*metricProcess)
		mProc.scrapped = true
		pMetric, err := prometheus.NewConstMetric(mProc.description, typeToPromType[mProc.metric.Type], mProc.metric.Value, mProc.metric.LabelVals...)
		if err != nil {
			pc.logger.Error("prometheus failed scrapping metric", err)
			return true
		}
		if pc.withtimestamp {
			if mProc.metric.Time == 0 {
				ch <- pMetric
				return true
			}
			ch <- prometheus.NewMetricWithTimestamp(time.Unix(int64(mProc.metric.Time), 0), pMetric)
			return true
		}
		ch <- pMetric
		return true
	})
}

// Dimensions return dimension size of labels in collector
func (pc *PromCollector) Dimensions() int {
	return pc.dimensions
}

// UpdateMetrics update metrics in collector
func (pc *PromCollector) UpdateMetrics(name string, time float64, typ data.MetricType, interval time.Duration, value float64, labelKeys []string, labelVals []string, ep *expiryProc) {
	var mProc *metricProcess
	pc.cacheindexbuilder.Grow(len(name))
	pc.cacheindexbuilder.WriteString(name)
	for _, v := range labelVals {
		pc.cacheindexbuilder.Grow(len(v))
		pc.cacheindexbuilder.WriteString(v)
	}
	cacheKey := pc.cacheindexbuilder.String()
	mProcItf, found := pc.mProc.Load(cacheKey)
	if !found {
		// pc.logger.Infof("creating new metric with cache key '%s'", cacheKey)
		mProcItf, _ = pc.mProc.LoadOrStore(cacheKey, &metricProcess{
			metric: &data.Metric{
				Name:      name,
				LabelKeys: labelKeys,
				LabelVals: labelVals,
				Time:      time,
				Type:      typ,
				Interval:  interval,
				Value:     value,
			},
			description: prometheus.NewDesc(name, "", labelKeys, nil),
			expiry: &metricExpiry{
				delete: func() bool {
					mp, ok := pc.mProc.Load(cacheKey)
					if !ok { // this should never happen
						pc.logger.Error("cache miss", errors.Newf("failed to locate '%s' in metric cache", cacheKey))
						return false
					}
					if mp.(*metricProcess).scrapped {
						pc.mProc.Delete(cacheKey)
						pc.logger.Debugf("metric '%s' expired after %.1fs of stale time", name, interval.Seconds())
						return true
					}
					return false
				},
			},
		})
		mProc = mProcItf.(*metricProcess)
		ep.register(mProc.expiry)
		mProc.expiry.keepAlive()
		pc.cacheindexbuilder.Reset()
		return
	}

	mProc = mProcItf.(*metricProcess)
	mProc.metric.Name = name
	mProc.metric.LabelKeys = labelKeys
	mProc.metric.LabelVals = labelVals
	mProc.metric.Time = time
	mProc.metric.Type = typ
	mProc.metric.Value = value
	mProc.expiry.keepAlive()
	pc.cacheindexbuilder.Reset()
}

// Prometheus plugin for interfacing with Prometheus. Metrics with the same dimensions
// are included in the same collectors even if the labels are different
type Prometheus struct {
	configuration       configT
	logger              *logWrapper
	collectors          sync.Map // collectors mapped according to label dimensions
	metricExpiryProcs   sync.Map // stores expiry processes based for each metric interval
	collectorExpiryProc *expiryProc
	registry            *prometheus.Registry
	ctx                 context.Context
	sync.RWMutex
}

// New constructor
func New(l *logging.Logger) application.Application {
	return &Prometheus{
		configuration: configT{
			Host:               "127.0.0.1",
			Port:               3000,
			ExpirationMultiple: 2,
		},
		logger: &logWrapper{
			l:      l,
			plugin: "Prometheus",
		},
		collectors:          sync.Map{},
		metricExpiryProcs:   sync.Map{},
		collectorExpiryProc: newExpiryProc(time.Duration(10) * time.Second),
	}
}

// ReceiveMetric callback function for receiving metric from the bus
func (p *Prometheus) ReceiveMetric(name string, t float64, typ data.MetricType, interval time.Duration, value float64, labelKeys []string, labelVals []string) {
	p.Lock()
	labelLen := len(labelKeys)
	var promCol *PromCollector

	pc, found := p.collectors.Load(labelLen)
	if !found {
		pc, _ = p.collectors.LoadOrStore(labelLen, NewPromCollector(p.logger, labelLen, p.configuration.WithTimestamp))
		promCol = pc.(*PromCollector)
		ce := &collectorExpiry{
			collector: promCol,
			delete: func() bool {
				p.logger.Warn("prometheus collector expired")
				p.registry.Unregister(promCol)
				p.collectors.Delete(len(labelKeys))
				return true
			},
		}
		numLabels := fmt.Sprintf("%d labels", labelLen)
		if labelLen == 1 {
			numLabels = "1 label"
		}
		p.collectorExpiryProc.register(ce)
		p.registry.MustRegister(promCol)
		p.logger.Infof("registered collector tracking metrics with %s", numLabels)
	} else {
		promCol = pc.(*PromCollector)
	}

	var expProc *expiryProc
	ep, found := p.metricExpiryProcs.Load(interval)
	if !found {
		ep, _ = p.metricExpiryProcs.LoadOrStore(interval, newExpiryProc(interval*time.Duration(p.configuration.ExpirationMultiple)))
		expProc = ep.(*expiryProc)
		p.logger.Infof("registered expiry process for metrics with interval %ds", interval/time.Second)
		go expProc.run(p.ctx)
	} else {
		expProc = ep.(*expiryProc)
	}

	promCol.UpdateMetrics(name, t, typ, interval, value, labelKeys, labelVals, expProc)
	p.Unlock()
}

// Run run scrape endpoint
func (p *Prometheus) Run(ctx context.Context, done chan bool) {
	p.ctx = ctx
	p.registry = prometheus.NewRegistry()

	// Set up Metric Exporter
	handler := http.NewServeMux()
	handler.Handle("/metrics", promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}))

	if p.configuration.ServeCache {
		handler.HandleFunc("/content", p.contentHandler)
	}
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
                                <head><title>Prometheus Exporter</title></head>
                                <body>cacheutil
                                <h1>Prometheus Exporter</h1>
                                <p><a href='/metrics'>Metrics</a></p>
                                </body>
								</html>`))
		if err != nil {
			p.logger.Error("HTTP error", err)
		}
	})

	// run exporter for prometheus to scrape
	metricsURL := fmt.Sprintf("%s:%d", p.configuration.Host, p.configuration.Port)

	srv := &http.Server{Addr: metricsURL}
	srv.Handler = handler

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			p.logger.Error("metric scrape endpoint failed", err)
			done <- true
		}
	}()

	p.logger.Infof("metric server at : %s", metricsURL)

	// run collector expiry process
	go p.collectorExpiryProc.run(ctx)

	<-ctx.Done()
	p.collectors.Range(func(key interface{}, value interface{}) bool {
		p.registry.Unregister(value.(*PromCollector))
		return true
	})
	timeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := srv.Shutdown(timeout); err != nil {
		p.logger.Error("error while shutting down metrics endpoint", err)
	}
	p.logger.Infof("exited")
}

// Config implements application.Application
func (p *Prometheus) Config(c []byte) error {
	err := config.ParseConfig(bytes.NewReader(c), &p.configuration)
	if err != nil {
		return err
	}

	return nil
}

// helper functions

func syncMapLen(m *sync.Map) int {
	len := 0
	m.Range(func(k interface{}, v interface{}) bool {
		len++
		return true
	})
	return len
}
