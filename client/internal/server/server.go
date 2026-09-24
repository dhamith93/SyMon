// Package server serves the dashboard and its JSON API. The API reads
// everything from the collector over gRPC.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/client/web"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/gorilla/handlers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type server struct {
	collector      api.MonitorDataServiceClient
	refreshSeconds int
	files          fs.FS
	// for the agent install script
	collectorEndpoint string
	agentCollector    string
	downloadsDir      string
}

// Run starts the server on the given address, like ":8080"
func Run(address string) {
	config := config.GetClient()

	conn, err := transport.Dial(config.CollectorEndpoint, config.CollectorEndpointCACertPath, transport.SharedKey())
	if err != nil {
		log.Fatal("cannot create collector client: ", err)
	}
	defer conn.Close()

	s := &server{
		collector:         api.NewMonitorDataServiceClient(conn),
		refreshSeconds:    config.RefreshSeconds,
		files:             web.Files(),
		collectorEndpoint: config.CollectorEndpoint,
		agentCollector:    config.AgentCollectorEndpoint,
		downloadsDir:      config.DownloadsDir,
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           handlers.CompressHandler(s.routes()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Log("info", "dashboard started on "+address)
	log.Fatal(httpServer.ListenAndServe())
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/config", s.getConfig)
	mux.HandleFunc("GET /api/v1/fleet", s.getFleet)
	mux.HandleFunc("GET /api/v1/hosts/{host}", s.getHost)
	mux.HandleFunc("GET /api/v1/hosts/{host}/series", s.getSeries)
	mux.HandleFunc("GET /api/v1/hosts/{host}/processes", s.getProcesses)
	mux.HandleFunc("GET /api/v1/hosts/{host}/custom-metrics", s.getCustomMetrics)
	mux.HandleFunc("GET /api/v1/alerts", s.getAlerts)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.HandleFunc("GET /install.sh", s.getInstallScript)
	mux.HandleFunc("GET /downloads/{file}", s.getDownload)
	mux.Handle("/", s.app())
	return mux
}

func (s *server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]int{"refreshSeconds": s.refreshSeconds})
}

type hostSummary struct {
	Name          string  `json:"name"`
	Up            bool    `json:"up"`
	LastSeen      int64   `json:"lastSeen"`
	Time          int64   `json:"time"`
	OS            string  `json:"os"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	CPUPct        float64 `json:"cpuPct"`
	MemUsedPct    float64 `json:"memUsedPct"`
	SwapUsedPct   float64 `json:"swapUsedPct"`
	DiskUsedPct   float64 `json:"diskUsedPct"`
	RxBps         float64 `json:"rxBps"`
	TxBps         float64 `json:"txBps"`
	ActiveAlerts  int32   `json:"activeAlerts"`
	WorstSeverity int32   `json:"worstSeverity"`
	Containers    int32   `json:"containers"`
}

func (s *server) getFleet(w http.ResponseWriter, r *http.Request) {
	fleet, err := s.collector.Fleet(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "fleet", err)
		return
	}
	hosts := make([]hostSummary, 0, len(fleet.Hosts))
	for _, h := range fleet.Hosts {
		hosts = append(hosts, hostSummary{
			Name:          h.Name,
			Up:            h.Up,
			LastSeen:      h.LastSeen,
			Time:          h.Time,
			OS:            h.Os,
			UptimeSeconds: h.UptimeSeconds,
			CPUPct:        h.CpuPct,
			MemUsedPct:    h.MemUsedPct,
			SwapUsedPct:   h.SwapUsedPct,
			DiskUsedPct:   h.DiskUsedPct,
			RxBps:         h.RxBps,
			TxBps:         h.TxBps,
			ActiveAlerts:  h.ActiveAlerts,
			WorstSeverity: h.WorstSeverity,
			Containers:    h.Containers,
		})
	}
	writeJSON(w, map[string]any{"hosts": hosts})
}

// getHost returns the host's latest snapshot as the agent sent it
func (s *server) getHost(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	snapshot, err := s.collector.Snapshot(r.Context(), &api.HostRequest{Host: host})
	if err != nil {
		writeGRPCError(w, "snapshot of "+host, err)
		return
	}
	writeJSON(w, map[string]any{
		"host":     snapshot.Host,
		"time":     snapshot.Time,
		"lastSeen": snapshot.LastSeen,
		"up":       snapshot.Up,
		"snapshot": json.RawMessage(snapshot.SnapshotJson),
	})
}

type series struct {
	Label string `json:"label"`
	// Points are [unix seconds, value] pairs
	Points [][2]float64 `json:"points"`
}

func (s *server) getSeries(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxPoints, _ := strconv.Atoi(query.Get("maxPoints"))

	response, err := s.collector.QuerySeries(r.Context(), &api.SeriesRequest{
		Host:      host,
		Metric:    query.Get("metric"),
		Label:     query.Get("label"),
		From:      from,
		To:        to,
		MaxPoints: int32(maxPoints),
		Max:       query.Get("max") == "1",
	})
	if err != nil {
		writeGRPCError(w, query.Get("metric")+" for "+host, err)
		return
	}

	out := make([]series, 0, len(response.Series))
	for _, s := range response.Series {
		points := make([][2]float64, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, [2]float64{float64(p.Time), p.Value})
		}
		out = append(out, series{Label: s.Label, Points: points})
	}
	writeJSON(w, map[string]any{
		"metric":      response.Metric,
		"source":      response.Source,
		"stepSeconds": response.StepSeconds,
		"series":      out,
	})
}

func (s *server) getProcesses(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	at, _ := strconv.ParseInt(r.URL.Query().Get("at"), 10, 64)
	response, err := s.collector.Processes(r.Context(), &api.ProcessesRequest{Host: host, At: at})
	if err != nil {
		writeGRPCError(w, "processes of "+host, err)
		return
	}
	writeJSON(w, map[string]any{
		"time":      response.Time,
		"processes": json.RawMessage(response.ProcessesJson),
	})
}

func (s *server) getCustomMetrics(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("host")
	names, err := s.collector.CustomMetricNames(r.Context(), &api.HostRequest{Host: host})
	if err != nil {
		writeGRPCError(w, "custom metrics of "+host, err)
		return
	}
	writeJSON(w, map[string]any{"names": nonNil(names.Names)})
}

type alert struct {
	ID         int64   `json:"id"`
	Host       string  `json:"host"`
	Rule       string  `json:"rule"`
	Metric     string  `json:"metric"`
	Target     string  `json:"target"`
	Severity   int32   `json:"severity"`
	Value      float64 `json:"value"`
	StartedAt  int64   `json:"startedAt"`
	UpdatedAt  int64   `json:"updatedAt"`
	ResolvedAt int64   `json:"resolvedAt"`
}

func (s *server) getAlerts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	request := &api.AlertsRequest{Host: query.Get("host"), OpenOnly: query.Get("open") == "1"}
	if query.Get("from") != "" || query.Get("to") != "" {
		from, to, err := timeRange(query.Get("from"), query.Get("to"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		request.From, request.To = from, to
	}

	response, err := s.collector.Alerts(r.Context(), request)
	if err != nil {
		writeGRPCError(w, "alerts", err)
		return
	}
	alerts := make([]alert, 0, len(response.Alerts))
	for _, a := range response.Alerts {
		alerts = append(alerts, alert{
			ID:         a.Id,
			Host:       a.Host,
			Rule:       a.Rule,
			Metric:     a.Metric,
			Target:     a.Target,
			Severity:   a.Severity,
			Value:      a.Value,
			StartedAt:  a.StartedAt,
			UpdatedAt:  a.UpdatedAt,
			ResolvedAt: a.ResolvedAt,
		})
	}
	writeJSON(w, map[string]any{"alerts": alerts})
}

// timeRange parses unix second from and to values. An empty to means now
// and an empty from means an hour before to.
func timeRange(fromValue string, toValue string) (int64, int64, error) {
	to := time.Now().Unix()
	if toValue != "" {
		parsed, err := strconv.ParseInt(toValue, 10, 64)
		if err != nil {
			return 0, 0, errors.New("to must be unix seconds")
		}
		to = parsed
	}
	from := to - 3600
	if fromValue != "" {
		parsed, err := strconv.ParseInt(fromValue, 10, 64)
		if err != nil {
			return 0, 0, errors.New("from must be unix seconds")
		}
		from = parsed
	}
	return from, to, nil
}

// app serves the built dashboard. Paths that are not files get index.html,
// so links into the app work.
func (s *server) app() http.Handler {
	fileServer := http.FileServerFS(s.files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if _, err := fs.Stat(s.files, name); err == nil {
				// built assets have hashed names, so they never change
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := fs.ReadFile(s.files, "index.html")
		if err != nil {
			http.Error(w, "the dashboard is not built, run make web", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Log("error", "cannot write response: "+err.Error())
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// writeGRPCError maps a collector error to an HTTP status. Only failures
// that are not the caller's fault are logged.
func writeGRPCError(w http.ResponseWriter, what string, err error) {
	st := status.Convert(err)
	switch st.Code() {
	case codes.Canceled:
		// the browser went away before the answer came back, 499 is the
		// usual "client closed request" status
		writeError(w, 499, "canceled")
	case codes.NotFound:
		writeError(w, http.StatusNotFound, st.Message())
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, st.Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		logger.Log("error", "cannot get "+what+": "+err.Error())
		writeError(w, http.StatusBadGateway, "the collector is not reachable")
	default:
		logger.Log("error", "cannot get "+what+": "+err.Error())
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func nonNil(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}
