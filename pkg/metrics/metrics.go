package metrics

import (
	"fmt"
	"io"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
	"github.com/wish/pod-netstat-exporter/pkg/netstat"
)

const (
	contentTypeHeader     = "Content-Type"
	contentEncodingHeader = "Content-Encoding"
)

// PodStats represents a pod and the metrics gathered for it
type PodStats struct {
	netstat.NetStats
	Name      string
	Namespace string
}

func s(s string) *string {
	return &s
}

func f(i int64) *float64 {
	f := float64(i)
	return &f
}

// generateMetrics creates the actual prometheus metrics from the raw pod stats
func generateMetrics(stats []*PodStats) []*dto.MetricFamily {
	timeMs := time.Now().Unix() * int64(time.Second/time.Millisecond)
	generateGaugeFamily := func(name, help string) *dto.MetricFamily {
		g := dto.MetricType_GAUGE
		return &dto.MetricFamily{
			Name:   &name,
			Help:   &help,
			Type:   &g,
			Metric: []*dto.Metric{},
		}
	}

	families := map[string]*dto.MetricFamily{}
	for _, podStat := range stats {
		for metricName, metricValue := range podStat.NetStats {
			family, ok := families["pod_netstat_"+metricName]
			if !ok {
				families["pod_netstat_"+metricName] = generateGaugeFamily(
					"pod_netstat_"+metricName,
					fmt.Sprintf("The per-pod value of the %v metric from /proc/net/(netstat|snmp|snmp6)", metricName),
				)
				family = families["pod_netstat_"+metricName]
			}
			family.Metric = append(family.Metric, &dto.Metric{
				Label: []*dto.LabelPair{
					&dto.LabelPair{Name: s("pod_namespace"), Value: &podStat.Namespace},
					&dto.LabelPair{Name: s("pod_name"), Value: &podStat.Name},
				},
				Gauge:       &dto.Gauge{Value: f(metricValue)},
				TimestampMs: &timeMs,
			})
		}
	}

	ret := []*dto.MetricFamily{}
	for _, metric := range families {
		ret = append(ret, metric)
	}
	return ret
}

// Handler returns metrics in response to an HTTP request
func Handler(rsp http.ResponseWriter, req *http.Request, stats []*PodStats) {
	logrus.Trace("Serving prometheus metrics")

	metrics := generateMetrics(stats)

	contentType := expfmt.Negotiate(req.Header)
	header := rsp.Header()
	header.Set(contentTypeHeader, string(contentType))
	w := io.Writer(rsp)
	enc := expfmt.NewEncoder(w, contentType)

	var lastErr error
	for _, mf := range metrics {
		if err := enc.Encode(mf); err != nil {
			lastErr = err
			HTTPError(rsp, err)
			return
		}
	}

	if lastErr != nil {
		HTTPError(rsp, lastErr)
	}
}

// HTTPError sends an error as an HTTP response
func HTTPError(rsp http.ResponseWriter, err error) {
	rsp.Header().Del(contentEncodingHeader)
	http.Error(
		rsp,
		"An error has occurred while serving metrics:\n\n"+err.Error(),
		http.StatusInternalServerError,
	)
}
