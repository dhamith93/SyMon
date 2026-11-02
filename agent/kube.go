package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/kube"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Kubernetes. With SYMON_KUBE_CLUSTER set, this agent reads the cluster's
// API with a read-only token and reports what it saw to the collector.
// One agent per cluster does this, the others only name their pods'
// containers from the CRI socket.

const (
	// how long one read of the API may take
	kubeReadTimeout = 20 * time.Second
	// how long to leave a collector from before clusters alone
	kubeOldCollector = 10 * time.Minute
)

type clusterClient interface {
	ReportCluster(ctx context.Context, in *api.ClusterReport, opts ...grpc.CallOption) (*api.Message, error)
}

type clusterReader struct {
	cluster string
	host    string
	reader  interface {
		Read(ctx context.Context, cluster string, now time.Time) kube.ClusterState
	}
	client clusterClient
	// lastError is logged once until the reader recovers
	lastError string
}

func newClusterReader(config *config.Agent, client clusterClient) (*clusterReader, error) {
	reader, err := kube.NewClient(config.KubeAPIURL, config.KubeCAPath, config.KubeTokenPath)
	if err != nil {
		return nil, err
	}
	return &clusterReader{cluster: config.KubeCluster, host: config.ServerId, reader: reader, client: client}, nil
}

func (r *clusterReader) loop() {
	logger.Log("info", "reading the kubernetes cluster "+r.cluster)
	for {
		time.Sleep(r.step())
	}
}

// step reads the cluster once and reports it. It returns how long to wait
// before the next step.
func (r *clusterReader) step() time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), kubeReadTimeout)
	state := r.reader.Read(ctx, r.cluster, time.Now())
	cancel()
	if state.Error != r.lastError {
		if state.Error != "" {
			logger.Log("error", "cannot read the kubernetes cluster "+r.cluster+": "+state.Error)
		} else {
			logger.Log("info", "reading the kubernetes cluster "+r.cluster+" again")
		}
		r.lastError = state.Error
	}

	data, err := json.Marshal(state)
	if err != nil {
		logger.Log("error", "cannot encode the cluster state: "+err.Error())
		return kube.ReportInterval
	}
	ctx, cancel = transport.Context()
	_, err = r.client.ReportCluster(ctx, &api.ClusterReport{Host: r.host, StateJson: string(data)})
	cancel()
	switch status.Code(err) {
	case codes.OK:
	case codes.Unimplemented, codes.PermissionDenied:
		// a collector from before clusters does not know the call, or does
		// not let agents make it
		logger.Log("error", "the collector is too old for kubernetes clusters, update it to see "+r.cluster)
		return kubeOldCollector
	default:
		logger.Log("error", "cannot report the kubernetes cluster: "+err.Error())
	}
	return kube.ReportInterval
}
