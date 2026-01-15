package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the server
type Metrics struct {
	blocksTotal       prometheus.Gauge
	manifestsTotal    prometheus.Gauge
	storageBytes      prometheus.Gauge
	bandwidthUpload   prometheus.Counter
	bandwidthDownload prometheus.Counter
}

// NewMetrics creates and registers all metrics
func NewMetrics() *Metrics {
	return &Metrics{
		blocksTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ib_blocks_total",
			Help: "Total number of blocks stored",
		}),
		manifestsTotal: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ib_manifests_total",
			Help: "Total number of manifests stored",
		}),
		storageBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ib_storage_bytes",
			Help: "Total storage used in bytes",
		}),
		bandwidthUpload: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ib_bandwidth_upload_bytes_total",
			Help: "Total bytes uploaded",
		}),
		bandwidthDownload: promauto.NewCounter(prometheus.CounterOpts{
			Name: "ib_bandwidth_download_bytes_total",
			Help: "Total bytes downloaded",
		}),
	}
}
