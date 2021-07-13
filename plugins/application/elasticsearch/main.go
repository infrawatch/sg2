package main

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/infrawatch/apputils/logging"
	"github.com/infrawatch/apputils/misc"
	"github.com/infrawatch/sg-core/pkg/application"
	"github.com/infrawatch/sg-core/pkg/config"
	"github.com/infrawatch/sg-core/pkg/data"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"

	"github.com/infrawatch/sg-core/plugins/application/elasticsearch/pkg/lib"
)

const (
	appname = "elasticsearch"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

// wrapper object for elasitcsearch index
type esIndex struct {
	index  string
	record []string
}

// used to marshal event into es usable json
type record struct {
	EventType   string                 `json:"event_type"`
	Generated   string                 `json:"generated"`
	Severity    string                 `json:"severity"`
	Labels      map[string]interface{} `json:"labels"`
	Annotations map[string]interface{} `json:"annotations"`
}

// used to marshal log into es usable json
type log struct {
	Timestamp string            `json:"@timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
}

// Elasticsearch plugin saves events to Elasticsearch database
type Elasticsearch struct {
	configuration *lib.AppConfig
	logger        *logging.Logger
	client        *lib.Client
	buffer        map[string][]string
	bufferMutex   sync.RWMutex
	dump          chan esIndex
}

// New constructor
func New(logger *logging.Logger) application.Application {
	return &Elasticsearch{
		logger: logger,
		buffer: make(map[string][]string),
		dump:   make(chan esIndex, 100),
	}
}

// ReceiveEvent receive event from event bus
func (es *Elasticsearch) ReceiveEvent(event data.Event) {
	var err error
	var record string
	switch event.Type {
	case data.EVENT:
		record, err = formatRecord(event)
	case data.LOG:
		record, err = formatLog(event)
	default:
		// eg. case data.TASK: this app does not respond on task request events
		//     case data.ERROR: TODO: save internal error
		//     case data.RESULT: TODO: save task result
		return
	}
	if err != nil {
		es.logger.Metadata(logging.Metadata{"plugin": appname, "event": event})
		es.logger.Error("failed formating record")
		return
	}

	// buffer or index record
	var recordList []string
	if es.configuration.BufferSize > 1 {
		es.bufferMutex.Lock()
		if _, ok := es.buffer[event.Index]; !ok {
			es.buffer[event.Index] = make([]string, 0, es.configuration.BufferSize)
		}

		es.buffer[event.Index] = append(es.buffer[event.Index], record)
		if len(es.buffer[event.Index]) < es.configuration.BufferSize {
			es.bufferMutex.Unlock()
			// buffer is not full, don't send
			es.logger.Metadata(logging.Metadata{"plugin": appname, "record": record})
			es.logger.Debug("buffering record")
			return
		}
		recordList = es.buffer[event.Index]
		delete(es.buffer, event.Index)
		es.bufferMutex.Unlock()

	} else {
		recordList = []string{record}
	}
	es.dump <- esIndex{index: event.Index, record: recordList}
}

// Run plugin process
func (es *Elasticsearch) Run(ctx context.Context, done chan bool) {
	es.logger.Metadata(logging.Metadata{"plugin": appname, "url": es.configuration.HostURL})
	es.logger.Info("storing events and(or) logs to Elasticsearch.")

	if es.configuration.ResetIndices != nil {
		err := es.client.IndicesDelete(es.configuration.ResetIndices)
		if err != nil {
			es.logger.Metadata(logging.Metadata{"plugin": appname, "error": err})
			es.logger.Error("failed removing indices")
			done <- true
			return
		}
		es.logger.Metadata(logging.Metadata{"plugin": appname, "indices": es.configuration.ResetIndices})
		es.logger.Info("removed indices")
	}

	wg := sync.WaitGroup{}
	for i := 0; i < es.configuration.IndexWorkers; i++ {
		es.logger.Metadata(logging.Metadata{"plugin": appname, "worker-id": i})
		es.logger.Debug("spawning ES API worker")
		wg.Add(1)

		go func(es *Elasticsearch, ctx context.Context, wg *sync.WaitGroup, i int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					es.logger.Metadata(logging.Metadata{"plugin": appname, "worker-id": i})
					es.logger.Debug("shutting down ES API worker")
					return
				case dumped := <-es.dump:
					if err := es.client.Index(dumped.index, dumped.record, es.configuration.BulkIndex); err != nil {
						es.logger.Metadata(logging.Metadata{"plugin": appname, "event": dumped.record, "error": err})
						es.logger.Error("failed to index event - disregarding")
					} else {
						es.logger.Debug("successfully indexed document(s)")
					}
				}
			}
		}(es, ctx, &wg, i)

	}

	wg.Wait()
	es.logger.Metadata(logging.Metadata{"plugin": appname})
	es.logger.Info("exited")
}

// Config implements application.Application
func (es *Elasticsearch) Config(c []byte) error {
	es.configuration = &lib.AppConfig{
		HostURL:       "",
		UseTLS:        false,
		TLSServerName: "",
		TLSClientCert: "",
		TLSClientKey:  "",
		TLSCaCert:     "",
		UseBasicAuth:  false,
		User:          "",
		Password:      "",
		BufferSize:    1,
		BulkIndex:     false,
		IndexWorkers:  3,
	}
	err := config.ParseConfig(bytes.NewReader(c), es.configuration)
	if err != nil {
		return err
	}

	if es.configuration.UseBasicAuth && !es.configuration.UseTLS {
		es.logger.Metadata(logging.Metadata{"plugin": appname})
		es.logger.Warn("insecure: using basic authentication without TLS enabled")
	}

	es.client, err = lib.NewElasticClient(es.configuration)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Elasticsearch host")
	}
	return nil
}

func formatRecord(e data.Event) (string, error) {
	record := record{
		EventType:   e.Type.String(),
		Generated:   timeFromEpoch(e.Time),
		Severity:    e.Severity.String(),
		Labels:      e.Labels,
		Annotations: e.Annotations,
	}

	res, err := json.Marshal(record)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

func formatLog(e data.Event) (string, error) {
	dest := map[string]string{}
	misc.AssimilateMap(misc.MergeMaps(e.Annotations, e.Labels), &dest)
	record := log{
		Timestamp: timeFromEpoch(e.Time),
		Labels:    dest,
		Message:   e.Message,
	}

	// correct severity value in labels
	record.Labels["severity"] = e.Severity.String()

	res, err := json.Marshal(record)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

// Get time in RFC3339
func timeFromEpoch(epoch float64) string {
	if epoch == 0.0 {
		return time.Now().Format(time.RFC3339)
	}
	whole := int64(epoch)
	t := time.Unix(whole, int64((float64(whole)-epoch)*1000000000))
	return t.Format(time.RFC3339)
}
