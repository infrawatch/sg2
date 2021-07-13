package main

import (
	stdjson "encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/infrawatch/apputils/logging"
	"github.com/infrawatch/sg-core/pkg/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testConf = `
hostURL: "http://localhost:9200"
useTLS:  false
bufferSize: 1
bulkIndex: false
resetIndices:
  - "unit-test"
`
)

type elasticTestCase struct {
	Event  data.Event
	Result map[string]interface{}
}

var (
	eventCases = []elasticTestCase{
		{
			Event: data.Event{
				Index:     "ceilometer_image",
				Time:      1583504009,
				Type:      data.EVENT,
				Publisher: "telemetry.publisher.controller-0.redhat.local",
				Severity:  data.INFO,
				Labels: map[string]interface{}{
					"service":     "image.localhost",
					"project_id":  "0f500647077b47f08a8ca9181e9b7aef",
					"user_id":     "0f500647077b47f08a8ca9181e9b7aef",
					"resource_id": "c4f7e00b-df85-4b77-9e1a-26a1de4d5735",
					"name":        "cirros",
					"status":      "deleted",
					"created_at":  "2020-03-06T14:01:07",
					"deleted_at":  "2020-03-06T14:13:29",
					"size":        13287936,
				},
				Annotations: map[string]interface{}{
					"source_type":  "ceilometer",
					"processed_by": "sg",
				},
			},
			Result: map[string]interface{}{
				"event_type": "event",
				"severity":   "info",
				"labels": map[string]interface{}{
					"created_at":  "2020-03-06T14:01:07",
					"deleted_at":  "2020-03-06T14:13:29",
					"name":        "cirros",
					"project_id":  "0f500647077b47f08a8ca9181e9b7aef",
					"resource_id": "c4f7e00b-df85-4b77-9e1a-26a1de4d5735",
					"service":     "image.localhost",
					"size":        float64(13287936),
					"status":      "deleted",
					"user_id":     "0f500647077b47f08a8ca9181e9b7aef",
				},
				"annotations": map[string]interface{}{
					"processed_by": "sg",
					"source_type":  "ceilometer",
				},
			},
		},
		{
			Event: data.Event{
				Index:     "collectd_elastic_check",
				Time:      1601900769,
				Type:      1,
				Publisher: "unknown",
				Severity:  data.CRITICAL,
				Labels: map[string]interface{}{
					"check":    "elastic-check",
					"client":   "wubba.lubba.dub.dub.redhat.com",
					"severity": "FAILURE",
				},
				Annotations: map[string]interface{}{
					"command":      "podman ps | grep elastic || exit 2",
					"duration":     int(1),
					"executed":     int(1601900769),
					"issued":       int(1601900769),
					"output":       "time=\"2020-10-05T14:26:09+02:00\" level=error msg=\"cannot mkdir /run/user/0/libpod: mkdir /run/user/0/libpod: permission denied\"\\n",
					"processed_by": "sg",
					"source_type":  "collectd",
					"status":       int(2),
					"ves": map[string]interface{}{
						"commonEventHeader": map[string]interface{}{
							"domain":                "heartbeat",
							"eventId":               "wubba.lubba.dub.dub.redhat.com-elastic-check",
							"eventType":             "checkResult",
							"lastEpochMicrosec":     int(1601900769),
							"priority":              "High",
							"reportingEntityId":     "918e8d04-c5ae-4e20-a763-8eb4f1af7c80",
							"reportingEntityName":   "wubba.lubba.dub.dub.redhat.com",
							"sourceId":              "918e8d04-c5ae-4e20-a763-8eb4f1af7c80",
							"sourceName":            "wubba.lubba.dub.dub.redhat.com-collectd-sensubility",
							"startingEpochMicrosec": int(1601900769),
						},
						"heartbeatFields": map[string]interface{}{
							"additionalFields": map[string]interface{}{
								"check":    "elastic-check",
								"command":  "podman ps | grep elastic || exit 2",
								"duration": "1",
								"executed": "1601900769",
								"issued":   "1601900769",
								"output":   "time=\"2020-10-05T14:26:09+02:00\" level=error msg=\"cannot mkdir /run/user/0/libpod: mkdir /run/user/0/libpod: permission denied\"\\n",
								"status":   "2",
							},
						},
					},
				},
				Message: "",
			},
			Result: map[string]interface{}{
				"event_type": "event",
				"severity":   "critical",
				"labels": map[string]interface{}{
					"check":    "elastic-check",
					"client":   "wubba.lubba.dub.dub.redhat.com",
					"severity": "FAILURE",
				},
				"annotations": map[string]interface{}{
					"command":      "podman ps | grep elastic || exit 2",
					"duration":     float64(1),
					"executed":     float64(1601900769),
					"issued":       float64(1601900769),
					"output":       "time=\"2020-10-05T14:26:09+02:00\" level=error msg=\"cannot mkdir /run/user/0/libpod: mkdir /run/user/0/libpod: permission denied\"\\n",
					"processed_by": "sg",
					"source_type":  "collectd",
					"status":       float64(2),
					"ves": map[string]interface{}{
						"commonEventHeader": map[string]interface{}{
							"domain":                "heartbeat",
							"eventId":               "wubba.lubba.dub.dub.redhat.com-elastic-check",
							"eventType":             "checkResult",
							"lastEpochMicrosec":     float64(1601900769),
							"priority":              "High",
							"reportingEntityId":     "918e8d04-c5ae-4e20-a763-8eb4f1af7c80",
							"reportingEntityName":   "wubba.lubba.dub.dub.redhat.com",
							"sourceId":              "918e8d04-c5ae-4e20-a763-8eb4f1af7c80",
							"sourceName":            "wubba.lubba.dub.dub.redhat.com-collectd-sensubility",
							"startingEpochMicrosec": float64(1601900769),
						},
						"heartbeatFields": map[string]interface{}{
							"additionalFields": map[string]interface{}{
								"check":    "elastic-check",
								"command":  "podman ps | grep elastic || exit 2",
								"duration": "1",
								"executed": "1601900769",
								"issued":   "1601900769",
								"output":   "time=\"2020-10-05T14:26:09+02:00\" level=error msg=\"cannot mkdir /run/user/0/libpod: mkdir /run/user/0/libpod: permission denied\"\\n",
								"status":   "2",
							},
						},
					},
				},
			},
		},
	}
	logCases = []elasticTestCase{
		{
			Event: data.Event{
				Index:     "logs-overcloud-controller0-2021-03-24",
				Time:      1616595773,
				Type:      data.LOG,
				Publisher: "overcloud-controller0",
				Severity:  data.CRITICAL,
				Labels: map[string]interface{}{
					"host":     "overcloud-controller0",
					"severity": "critical",
					"facility": "local0",
					"tag":      "openstack.nova",
					"source":   "openstack-nova-conductor",
					"file":     "/var/log/nova/nova-conductor.log",
					"cloud":    "overcloud",
					"region":   "regionOne",
				},
				Message: "2021-03-24 14:22:53.063 16 ERROR stevedore.extension [req-58ef54fc-79a2-4fb1-9b53-f63d21cb3343 " +
					"4d249f1635374d4b915f2f181caf9b43 81c09cd4e8f5456f9c196a53afb58c8d - default default] Could not load 'oslo_cache.etcd3gw': " +
					"No module named 'etcd3gw': ModuleNotFoundError: No module named 'etcd3gw'",
			},
			Result: map[string]interface{}{
				"labels": map[string]interface{}{
					"cloud":    "overcloud",
					"facility": "local0",
					"file":     "/var/log/nova/nova-conductor.log",
					"host":     "overcloud-controller0",
					"region":   "regionOne",
					"severity": "critical",
					"source":   "openstack-nova-conductor",
					"tag":      "openstack.nova",
				},
				"message": "2021-03-24 14:22:53.063 16 ERROR stevedore.extension [req-58ef54fc-79a2-4fb1-9b53-f63d21cb3343 " +
					"4d249f1635374d4b915f2f181caf9b43 81c09cd4e8f5456f9c196a53afb58c8d - default default] Could not load 'oslo_cache.etcd3gw': " +
					"No module named 'etcd3gw': ModuleNotFoundError: No module named 'etcd3gw'",
			},
		},
	}
)

func TestElasticsearchApp(t *testing.T) {
	tmpdir, err := ioutil.TempDir(".", "elastic_test_tmp")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	logpath := path.Join(tmpdir, "test.log")
	logger, err := logging.NewLogger(logging.DEBUG, logpath)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, logger.Destroy())
	}()

	t.Run("Test configuration", func(t *testing.T) {
		app := New(logger)
		err := app.Config([]byte(testConf))
		require.NoError(t, err)

		// test parsed and default values
		es := app.(*Elasticsearch)
		assert.Equal(t, "http://localhost:9200", es.configuration.HostURL)
		assert.Equal(t, false, es.configuration.UseTLS)
		assert.Equal(t, "", es.configuration.TLSServerName)
		assert.Equal(t, "", es.configuration.TLSClientCert)
		assert.Equal(t, "", es.configuration.TLSClientKey)
		assert.Equal(t, "", es.configuration.TLSCaCert)
		assert.Equal(t, false, es.configuration.UseBasicAuth)
		assert.Equal(t, "", es.configuration.User)
		assert.Equal(t, "", es.configuration.Password)
		assert.Equal(t, 1, es.configuration.BufferSize)
		assert.Equal(t, false, es.configuration.BulkIndex)
		assert.Equal(t, 3, es.configuration.IndexWorkers)
		assert.Equal(t, []string{"unit-test"}, es.configuration.ResetIndices)
	})

	t.Run("Test event message processing", func(t *testing.T) {
		results := make(chan esIndex, len(eventCases))
		app := &Elasticsearch{
			logger: logger,
			buffer: make(map[string][]string),
			dump:   results,
		}
		err := app.Config([]byte(testConf))
		require.NoError(t, err)

		for _, tstCase := range eventCases {
			app.ReceiveEvent(tstCase.Event)
			res := <-results

			var result map[string]interface{}
			require.NoError(t, stdjson.Unmarshal([]byte(res.record[0]), &result))
			assert.EqualValues(t, tstCase.Result["labels"], result["labels"])
			assert.EqualValues(t, tstCase.Result["annotations"], result["annotations"])
			assert.EqualValues(t, tstCase.Result["severity"], result["severity"])
		}
	})

	t.Run("Test log message processing", func(t *testing.T) {
		results := make(chan esIndex, len(logCases))
		app := &Elasticsearch{
			logger: logger,
			buffer: make(map[string][]string),
			dump:   results,
		}
		err := app.Config([]byte(testConf))
		require.NoError(t, err)

		for _, tstCase := range logCases {
			app.ReceiveEvent(tstCase.Event)
			res := <-results

			var result map[string]interface{}
			require.NoError(t, stdjson.Unmarshal([]byte(res.record[0]), &result))
			assert.EqualValues(t, tstCase.Result["labels"], result["labels"])
			assert.EqualValues(t, tstCase.Result["message"], result["message"])
			assert.Contains(t, result, "@timestamp")
		}
	})
}
