// arris_cm_exporter, a Prometheus exporter for Arris Cable Modems
// Copyright 2021 Mark Stenglein
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	listenAddress = flag.String("web.listen-address", ":9143",
		"Address to listen on for telemetry")
	metricsPath = flag.String("web.telemetry-path", "/metrics",
		"Path under which to expose metrics")
)

func main() {
	host := os.Getenv("ARRIS_CM_HOST")
	user := "admin"
	password := os.Getenv("ARRIS_CM_PASSWORD")

	exporter := NewExporter(host, user, password)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
		<head><title>Arris Cable Modem Exporter</title></head>
		<body>
		<h1>SB8200 Exporter</h1>
		<p><a href='` + *metricsPath + `'>Metrics</a></p>
		</body>
		</html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, handlers.LoggingHandler(os.Stdout, http.DefaultServeMux)))
}
