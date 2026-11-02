package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/api"
)

// Kubernetes clusters, as the agent that reads each reports it. Viewers see
// all of it, there are no secrets or pod specs in a state.

type clusterSummary struct {
	Name             string `json:"name"`
	Host             string `json:"host"`
	StateTime        int64  `json:"stateTime"`
	ReportedAt       int64  `json:"reportedAt"`
	Error            string `json:"error"`
	Stale            bool   `json:"stale"`
	Nodes            int32  `json:"nodes"`
	NodesReady       int32  `json:"nodesReady"`
	Pods             int32  `json:"pods"`
	PodsRunning      int32  `json:"podsRunning"`
	PodsPending      int32  `json:"podsPending"`
	PodsNotReady     int32  `json:"podsNotReady"`
	PodsFailed       int32  `json:"podsFailed"`
	Workloads        int32  `json:"workloads"`
	WorkloadsReady   int32  `json:"workloadsReady"`
	RestartsLastHour int32  `json:"restartsLastHour"`
}

func summaryOf(c *api.ClusterSummary) clusterSummary {
	return clusterSummary{
		Name:             c.Name,
		Host:             c.Host,
		StateTime:        c.StateTime,
		ReportedAt:       c.ReportedAt,
		Error:            c.Error,
		Stale:            c.Stale,
		Nodes:            c.Nodes,
		NodesReady:       c.NodesReady,
		Pods:             c.Pods,
		PodsRunning:      c.PodsRunning,
		PodsPending:      c.PodsPending,
		PodsNotReady:     c.PodsNotReady,
		PodsFailed:       c.PodsFailed,
		Workloads:        c.Workloads,
		WorkloadsReady:   c.WorkloadsReady,
		RestartsLastHour: c.RestartsLastHour,
	}
}

func (s *server) getClusters(w http.ResponseWriter, r *http.Request) {
	response, err := s.collector.Clusters(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "clusters", err)
		return
	}
	clusters := make([]clusterSummary, 0, len(response.Clusters))
	for _, c := range response.Clusters {
		clusters = append(clusters, summaryOf(c))
	}
	writeJSON(w, map[string]any{"clusters": clusters})
}

type containerUsage struct {
	ID          string  `json:"id"`
	Host        string  `json:"host"`
	CPUPercent  float64 `json:"cpuPercent"`
	CPUCores    float64 `json:"cpuCores"`
	MemoryBytes float64 `json:"memoryBytes"`
	MemoryLimit float64 `json:"memoryLimit"`
}

type kubeRestart struct {
	Time      int64  `json:"time"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Count     int32  `json:"count"`
	Reason    string `json:"reason"`
}

// getCluster returns the newest state as the agent sent it, with the pods'
// usage from the nodes' agents
func (s *server) getCluster(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	info, err := s.collector.Cluster(r.Context(), &api.ClusterRequest{Name: name})
	if err != nil {
		writeGRPCError(w, "cluster "+name, err)
		return
	}
	usage := make([]containerUsage, 0, len(info.Usage))
	for _, u := range info.Usage {
		usage = append(usage, containerUsage{ID: u.Id, Host: u.Host, CPUPercent: u.CpuPercent, CPUCores: u.CpuCores, MemoryBytes: u.MemoryBytes, MemoryLimit: u.MemoryLimit})
	}
	restarts := make([]kubeRestart, 0, len(info.Restarts))
	for _, k := range info.Restarts {
		restarts = append(restarts, kubeRestart{Time: k.Time, Namespace: k.Namespace, Workload: k.Workload, Pod: k.Pod, Container: k.Container, Count: k.Count, Reason: k.Reason})
	}
	hosts := info.Hosts
	if hosts == nil {
		hosts = []string{}
	}
	writeJSON(w, map[string]any{
		"summary":  summaryOf(info.Summary),
		"state":    json.RawMessage(info.StateJson),
		"usage":    usage,
		"restarts": restarts,
		"hosts":    hosts,
	})
}

func (s *server) getClusterSeries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	query := r.URL.Query()
	from, to, err := timeRange(query.Get("from"), query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxPoints, _ := strconv.Atoi(query.Get("maxPoints"))
	response, err := s.collector.ClusterSeries(r.Context(), &api.ClusterSeriesRequest{
		Name:      name,
		Metric:    query.Get("metric"),
		From:      from,
		To:        to,
		MaxPoints: int32(maxPoints),
	})
	if err != nil {
		writeGRPCError(w, query.Get("metric")+" for cluster "+name, err)
		return
	}
	writeSeries(w, response)
}

// deleteCluster forgets a cluster, so another host's agent may report it
func (s *server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.collector.RemoveCluster(r.Context(), &api.ClusterRequest{Name: name}); err != nil {
		writeGRPCError(w, "removing cluster "+name, err)
		return
	}
	writeJSON(w, map[string]string{"name": name})
}
