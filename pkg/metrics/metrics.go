/*
 * Copyright 2020 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/version"
)

var (
	ActiveBrowserSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNs,
		Subsystem: metricsSubsystem,
		Name:      "active_browser_sessions",
		Help:      "Active browser sessions",
	})

	BrowserSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNs,
		Subsystem: metricsSubsystem,
		Name:      "browser_sessions",
		Help:      "Available browser sessions",
	})

	PagesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNs,
		Subsystem: metricsSubsystem,
		Name:      "pages_total",
		Help:      "Total pages processed",
	})

	PagesFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNs,
		Subsystem: metricsSubsystem,
		Name:      "pages_failed_total",
		Help:      "Total pages failed processing",
	},
		[]string{"code"},
	)

	PageFetchSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNs,
		Subsystem: metricsSubsystem,
		Name:      "pages_fetch_seconds",
		Help:      "Time for fetching a complete page in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .075, .1, .25, .5, .75, 1, 2.5, 5, 7.5, 10, 20, 30, 40, 50, 60, 120, 180, 240},
	})
)

func init() {
	prometheus.MustRegister(version.NewCollector("veidemann_browser_controller"))
}

const (
	metricsNs        = "veidemann"
	metricsSubsystem = "harvester"
)
