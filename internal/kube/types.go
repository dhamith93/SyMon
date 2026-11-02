// Package kube reads a Kubernetes cluster's state from its API server. One
// agent in the cluster reads it and sends it to the collector, which keeps
// the newest state, counts for charts and the restarts it saw.
package kube

import "time"

// ReportInterval is how often the reader reads the cluster. The collector
// takes a state older than StaleAfter as no data.
const (
	ReportInterval = 30 * time.Second
	StaleAfter     = 3 * ReportInterval
)

// ClusterState is what the reader saw at one time. Only names, states and
// counts are read, never a pod's spec, env or commands.
type ClusterState struct {
	Cluster string
	// Time is when it was read, in unix seconds
	Time int64
	// Error is why the API could not be read. The lists are then empty.
	Error     string `json:",omitempty"`
	Nodes     []Node
	Workloads []Workload
	Pods      []Pod
	// Events are the warning events of the last hour, newest first
	Events []Event
}

type Node struct {
	Name  string
	Ready bool
	// ReadyChanged is when Ready last changed, in unix seconds
	ReadyChanged   int64
	Roles          []string `json:",omitempty"`
	KubeletVersion string
	Unschedulable  bool
	// Pressure lists the pressure conditions that are on, like
	// MemoryPressure or DiskPressure
	Pressure []string `json:",omitempty"`
	// CPUCores and MemoryBytes are what pods may use
	CPUCores    float64
	MemoryBytes float64
}

// Workload is a deployment, statefulset or daemonset
type Workload struct {
	Namespace string
	Kind      string
	Name      string
	Desired   int
	Ready     int
	Available int
}

// Key is namespace/kind/name, like media/deployment/jellyfin, which alerts
// use as their target
func (w Workload) Key() string {
	return w.Namespace + "/" + w.Kind + "/" + w.Name
}

// Missing is how many of the wanted replicas are not available
func (w Workload) Missing() int {
	return max(w.Desired-w.Available, 0)
}

type Pod struct {
	Namespace string
	Name      string
	UID       string
	Node      string `json:",omitempty"`
	// Workload is kind/name of what runs the pod, like deployment/web, or
	// pod/<name> for a pod of its own
	Workload string
	// Phase is Pending, Running, Succeeded, Failed or Unknown
	Phase string
	Ready bool
	// Reason says why a pod is not running well, like CrashLoopBackOff,
	// ImagePullBackOff, Unschedulable or OOMKilled. Empty when it is fine.
	Reason string `json:",omitempty"`
	// Created is in unix seconds
	Created    int64
	Containers []PodContainer
}

// WorkloadKey is namespace/kind/name of the pod's workload
func (p Pod) WorkloadKey() string {
	return p.Namespace + "/" + p.Workload
}

// Restarts adds up its containers' restarts
func (p Pod) Restarts() int {
	total := 0
	for _, c := range p.Containers {
		total += c.Restarts
	}
	return total
}

type PodContainer struct {
	Name string
	// ID is the runtime's container ID, the one the node's agent reports
	ID       string `json:",omitempty"`
	Image    string
	Ready    bool
	Restarts int
	// State is running, waiting or terminated, and Reason why it waits or
	// ended
	State  string
	Reason string `json:",omitempty"`
	// LastReason and LastFinished are about the run before this one, like
	// OOMKilled. LastFinished is in unix seconds.
	LastReason   string `json:",omitempty"`
	LastFinished int64  `json:",omitempty"`
}

type Event struct {
	Namespace string
	// Kind and Name are the object the event is about
	Kind    string
	Name    string
	Reason  string
	Message string
	Count   int
	// LastSeen is in unix seconds
	LastSeen int64
}
