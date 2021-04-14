package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (p *Prometheus) contentHandler(w http.ResponseWriter, r *http.Request) {

	var sb strings.Builder
	p.collectors.Range(func(key, value interface{}) bool {
		c := value.(*PromCollector)
		c.mProc.Range(func(key, value interface{}) bool {
			mp := value.(*metricProcess)
			lastArrival := mp.expiry.lastArrival
			sb.WriteString(mp.metric.Name)
			sb.WriteString(" last-arrival=")
			sb.WriteString(fmt.Sprintf("%fs", time.Since(lastArrival).Seconds()))
			sb.WriteString(" interval=")
			sb.WriteString(fmt.Sprintf("%fs", mp.metric.Interval.Seconds()))
			sb.WriteString(" [")
			for index, labelKey := range mp.metric.LabelKeys {
				sb.WriteString(" ")
				sb.WriteString(labelKey)
				sb.WriteString("=")
				sb.WriteString(mp.metric.LabelVals[index])
			}
			sb.WriteString(" ]")
			sb.WriteString("\n")
			return true
		})

		return true
	})
	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		p.logger.Error("failed writing http response", err)
	}
}
