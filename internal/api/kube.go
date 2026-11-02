package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dhamith93/SyMon/internal/kube"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// clusterRestartsShown is how far back the cluster page lists restarts
const clusterRestartsShown = 24 * time.Hour

func (s *Server) ReportCluster(ctx context.Context, in *ClusterReport) (*Message, error) {
	var state kube.ClusterState
	if err := json.Unmarshal([]byte(in.StateJson), &state); err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot parse the cluster state: "+err.Error())
	}
	host := in.Host
	if agent, ok := agentHost(ctx); ok {
		host = agent
	}
	if err := s.Store.SaveClusterReport(ctx, host, state); err != nil {
		return nil, agentStatus(host, err)
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) Clusters(ctx context.Context, in *Void) (*ClusterList, error) {
	clusters, err := s.Store.Clusters(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	list := &ClusterList{}
	for _, cluster := range clusters {
		summary, err := s.clusterSummary(ctx, cluster)
		if err != nil {
			return nil, toStatus(err)
		}
		list.Clusters = append(list.Clusters, summary)
	}
	return list, nil
}

func (s *Server) clusterSummary(ctx context.Context, cluster store.Cluster) (*ClusterSummary, error) {
	now := time.Now()
	restarts, err := s.Store.RestartsSince(ctx, cluster.Name, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	total := 0
	for _, count := range restarts {
		total += count
	}
	counts := cluster.State.Counts()
	return &ClusterSummary{
		Name:             cluster.Name,
		Host:             cluster.Host,
		StateTime:        cluster.State.Time,
		ReportedAt:       cluster.ReportedAt.Unix(),
		Error:            cluster.Error,
		Stale:            cluster.Stale(now),
		Nodes:            int32(counts.Nodes),
		NodesReady:       int32(counts.NodesReady),
		Pods:             int32(counts.Pods),
		PodsRunning:      int32(counts.PodsRunning),
		PodsPending:      int32(counts.PodsPending),
		PodsNotReady:     int32(counts.PodsNotReady),
		PodsFailed:       int32(counts.PodsFailed),
		Workloads:        int32(counts.Workloads),
		WorkloadsReady:   int32(counts.WorkloadsReady),
		RestartsLastHour: int32(total),
	}, nil
}

// Cluster returns a cluster's newest state with what the nodes' agents say
// about its containers
func (s *Server) Cluster(ctx context.Context, in *ClusterRequest) (*ClusterInfo, error) {
	cluster, err := s.Store.Cluster(ctx, in.Name)
	if err != nil {
		return nil, toStatus(err)
	}
	summary, err := s.clusterSummary(ctx, cluster)
	if err != nil {
		return nil, toStatus(err)
	}
	state, err := json.Marshal(cluster.State)
	if err != nil {
		return nil, toStatus(err)
	}
	info := &ClusterInfo{Summary: summary, StateJson: string(state)}

	restarts, err := s.Store.RecentRestarts(ctx, cluster.Name, time.Now().Add(-clusterRestartsShown), 200)
	if err != nil {
		return nil, toStatus(err)
	}
	for _, r := range restarts {
		info.Restarts = append(info.Restarts, &KubeRestart{
			Time:      r.Time.Unix(),
			Namespace: r.Namespace,
			Workload:  r.Workload,
			Pod:       r.Pod,
			Container: r.Container,
			Count:     int32(r.Count),
			Reason:    r.Reason,
		})
	}

	ids := map[string]bool{}
	for _, pod := range cluster.State.Pods {
		for _, c := range pod.Containers {
			if c.ID != "" {
				ids[c.ID] = true
			}
		}
	}
	snapshots, err := s.Store.LatestSnapshots(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	for _, snapshot := range snapshots {
		info.Hosts = append(info.Hosts, snapshot.Host)
		info.Usage = append(info.Usage, containerUsage(snapshot, ids)...)
	}
	return info, nil
}

// containerUsage picks the containers in ids out of a host's snapshot
func containerUsage(snapshot store.LatestSnapshot, ids map[string]bool) []*ContainerUsage {
	if len(snapshot.Snapshot) == 0 || len(ids) == 0 {
		return nil
	}
	var data struct {
		Containers []monitor.Container
	}
	if err := json.Unmarshal(snapshot.Snapshot, &data); err != nil {
		logger.Log("error", "cannot read the containers of "+snapshot.Host+": "+err.Error())
		return nil
	}
	var usage []*ContainerUsage
	for _, c := range data.Containers {
		if !ids[c.ID] {
			continue
		}
		u := &ContainerUsage{
			Id:          c.ID,
			Host:        snapshot.Host,
			CpuPercent:  c.CPU.PercentOfHost,
			CpuCores:    c.CPU.CoresUsed,
			MemoryBytes: c.Memory.Used,
		}
		if c.Memory.Limited {
			u.MemoryLimit = c.Memory.Limit
		}
		usage = append(usage, u)
	}
	return usage
}

func (s *Server) ClusterSeries(ctx context.Context, in *ClusterSeriesRequest) (*SeriesResponse, error) {
	result, err := s.Store.ClusterSeries(ctx, in.Name, in.Metric, time.Unix(in.From, 0), time.Unix(in.To, 0), int(in.MaxPoints))
	if err != nil {
		return nil, toStatus(err)
	}
	return seriesResponse(in.Metric, result), nil
}

func (s *Server) RemoveCluster(ctx context.Context, in *ClusterRequest) (*Message, error) {
	if err := s.Store.RemoveCluster(ctx, in.Name); err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "cluster "+in.Name+" removed")
	return &Message{Body: "ok"}, nil
}
