package kube

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// the parts of API objects the reader uses

type objectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	OwnerReferences   []ownerReference  `json:"ownerReferences"`
}

type ownerReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller bool   `json:"controller"`
}

type condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

func findCondition(conditions []condition, kind string) (condition, bool) {
	i := slices.IndexFunc(conditions, func(c condition) bool { return c.Type == kind })
	if i < 0 {
		return condition{}, false
	}
	return conditions[i], true
}

type apiNode struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Unschedulable bool `json:"unschedulable"`
	} `json:"spec"`
	Status struct {
		Conditions  []condition       `json:"conditions"`
		Allocatable map[string]string `json:"allocatable"`
		NodeInfo    struct {
			KubeletVersion string `json:"kubeletVersion"`
		} `json:"nodeInfo"`
	} `json:"status"`
}

// role labels look like node-role.kubernetes.io/control-plane
const roleLabel = "node-role.kubernetes.io/"

func (n apiNode) node() Node {
	node := Node{
		Name:           n.Metadata.Name,
		KubeletVersion: n.Status.NodeInfo.KubeletVersion,
		Unschedulable:  n.Spec.Unschedulable,
		CPUCores:       quantity(n.Status.Allocatable["cpu"]),
		MemoryBytes:    quantity(n.Status.Allocatable["memory"]),
	}
	if ready, ok := findCondition(n.Status.Conditions, "Ready"); ok {
		node.Ready = ready.Status == "True"
		node.ReadyChanged = unix(ready.LastTransitionTime)
	}
	for _, c := range n.Status.Conditions {
		if strings.HasSuffix(c.Type, "Pressure") && c.Status == "True" {
			node.Pressure = append(node.Pressure, c.Type)
		}
	}
	for label := range n.Metadata.Labels {
		if role, ok := strings.CutPrefix(label, roleLabel); ok && role != "" {
			node.Roles = append(node.Roles, role)
		}
	}
	slices.Sort(node.Roles)
	return node
}

// apiReplicated is a deployment or a statefulset
type apiReplicated struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		// nil means 1
		Replicas *int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

func (r apiReplicated) workload(kind string) Workload {
	desired := 1
	if r.Spec.Replicas != nil {
		desired = *r.Spec.Replicas
	}
	return Workload{
		Namespace: r.Metadata.Namespace,
		Kind:      kind,
		Name:      r.Metadata.Name,
		Desired:   desired,
		Ready:     r.Status.ReadyReplicas,
		Available: r.Status.AvailableReplicas,
	}
}

type apiDaemonSet struct {
	Metadata objectMeta `json:"metadata"`
	Status   struct {
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		NumberAvailable        int `json:"numberAvailable"`
	} `json:"status"`
}

func (d apiDaemonSet) workload() Workload {
	return Workload{
		Namespace: d.Metadata.Namespace,
		Kind:      "daemonset",
		Name:      d.Metadata.Name,
		Desired:   d.Status.DesiredNumberScheduled,
		Ready:     d.Status.NumberReady,
		Available: d.Status.NumberAvailable,
	}
}

type apiPod struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase                 string            `json:"phase"`
		Reason                string            `json:"reason"`
		Conditions            []condition       `json:"conditions"`
		InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		ContainerStatuses     []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type containerStatus struct {
	Name         string         `json:"name"`
	Image        string         `json:"image"`
	ContainerID  string         `json:"containerID"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	State        containerState `json:"state"`
	LastState    containerState `json:"lastState"`
}

type containerState struct {
	Waiting *struct {
		Reason string `json:"reason"`
	} `json:"waiting"`
	Running    *struct{} `json:"running"`
	Terminated *struct {
		Reason     string    `json:"reason"`
		FinishedAt time.Time `json:"finishedAt"`
	} `json:"terminated"`
}

func (p apiPod) pod() Pod {
	pod := Pod{
		Namespace: p.Metadata.Namespace,
		Name:      p.Metadata.Name,
		UID:       p.Metadata.UID,
		Node:      p.Spec.NodeName,
		Workload:  podWorkload(p.Metadata),
		Phase:     p.Status.Phase,
		Created:   unix(p.Metadata.CreationTimestamp),
	}
	if ready, ok := findCondition(p.Status.Conditions, "Ready"); ok {
		pod.Ready = ready.Status == "True"
	}
	for _, s := range p.Status.ContainerStatuses {
		pod.Containers = append(pod.Containers, s.container())
	}
	pod.Reason = p.reason()
	return pod
}

func (s containerStatus) container() PodContainer {
	c := PodContainer{
		Name:     s.Name,
		Image:    s.Image,
		Ready:    s.Ready,
		Restarts: s.RestartCount,
	}
	// containerd://<id>, the agent on the node reports the bare id
	if _, id, ok := strings.Cut(s.ContainerID, "://"); ok {
		c.ID = id
	}
	switch {
	case s.State.Running != nil:
		c.State = "running"
	case s.State.Waiting != nil:
		c.State, c.Reason = "waiting", s.State.Waiting.Reason
	case s.State.Terminated != nil:
		c.State, c.Reason = "terminated", s.State.Terminated.Reason
	}
	if last := s.LastState.Terminated; last != nil {
		c.LastReason = last.Reason
		c.LastFinished = unix(last.FinishedAt)
	}
	return c
}

// waits that are part of starting normally
var startingReasons = map[string]bool{"ContainerCreating": true, "PodInitializing": true}

// reason says why a pod is not running well, the way kubectl's STATUS
// column does. Empty when nothing is wrong.
func (p apiPod) reason() string {
	if p.Status.Phase == "Succeeded" {
		return ""
	}
	if p.Status.Phase == "Failed" && p.Status.Reason != "" {
		// like Evicted
		return p.Status.Reason
	}
	if scheduled, ok := findCondition(p.Status.Conditions, "PodScheduled"); ok && scheduled.Status == "False" && scheduled.Reason != "" {
		return scheduled.Reason
	}
	for _, s := range p.Status.InitContainerStatuses {
		if s.State.Waiting != nil && s.State.Waiting.Reason != "" && !startingReasons[s.State.Waiting.Reason] {
			return "Init:" + s.State.Waiting.Reason
		}
	}
	for _, s := range p.Status.ContainerStatuses {
		if s.State.Waiting != nil && s.State.Waiting.Reason != "" && !startingReasons[s.State.Waiting.Reason] {
			return s.State.Waiting.Reason
		}
	}
	for _, s := range p.Status.ContainerStatuses {
		if s.State.Terminated != nil && s.State.Terminated.Reason != "" && s.State.Terminated.Reason != "Completed" {
			return s.State.Terminated.Reason
		}
	}
	return ""
}

// podWorkload is kind/name of what runs a pod. A ReplicaSet's pods belong
// to its deployment, whose name is the ReplicaSet's without the pod
// template hash. Static pods, owned by their node, are pods of their own.
func podWorkload(meta objectMeta) string {
	i := slices.IndexFunc(meta.OwnerReferences, func(o ownerReference) bool { return o.Controller })
	if i < 0 {
		return "pod/" + meta.Name
	}
	owner := meta.OwnerReferences[i]
	switch owner.Kind {
	case "ReplicaSet":
		if hash := meta.Labels["pod-template-hash"]; hash != "" {
			if deployment, ok := strings.CutSuffix(owner.Name, "-"+hash); ok {
				return "deployment/" + deployment
			}
		}
	case "Node":
		return "pod/" + meta.Name
	}
	return strings.ToLower(owner.Kind) + "/" + owner.Name
}

type apiEvent struct {
	Metadata       objectMeta `json:"metadata"`
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"involvedObject"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
	Count         int       `json:"count"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	EventTime     time.Time `json:"eventTime"`
	Series        *struct {
		Count            int       `json:"count"`
		LastObservedTime time.Time `json:"lastObservedTime"`
	} `json:"series"`
}

// eventWindow and maxEvents bound the warning events in a state
const (
	eventWindow = time.Hour
	maxEvents   = 50
)

// warningEvents keeps the events seen in the last hour, newest first
func warningEvents(events []apiEvent, now time.Time) []Event {
	var output []Event
	for _, e := range events {
		last, count := e.LastTimestamp, e.Count
		if e.EventTime.After(last) {
			last = e.EventTime
		}
		if e.Series != nil {
			if e.Series.LastObservedTime.After(last) {
				last = e.Series.LastObservedTime
			}
			count = max(count, e.Series.Count)
		}
		if now.Sub(last) > eventWindow {
			continue
		}
		namespace := e.InvolvedObject.Namespace
		if namespace == "" {
			namespace = e.Metadata.Namespace
		}
		message := e.Message
		if len(message) > 300 {
			message = message[:300] + "…"
		}
		output = append(output, Event{
			Namespace: namespace,
			Kind:      e.InvolvedObject.Kind,
			Name:      e.InvolvedObject.Name,
			Reason:    e.Reason,
			Message:   message,
			Count:     max(count, 1),
			LastSeen:  last.Unix(),
		})
	}
	slices.SortStableFunc(output, func(a, b Event) int { return cmp.Compare(b.LastSeen, a.LastSeen) })
	if len(output) > maxEvents {
		output = output[:maxEvents]
	}
	return output
}

// quantity reads a resource quantity like 3800m, 4 or 8023488Ki. Anything
// it does not know reads as 0.
func quantity(q string) float64 {
	// milli divides, 3800 * 0.001 is not quite 3.8
	if number, ok := strings.CutSuffix(q, "m"); ok {
		value, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0
		}
		return value / 1000
	}
	suffixes := []struct {
		suffix string
		factor float64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40},
		{"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12},
	}
	for _, s := range suffixes {
		if number, ok := strings.CutSuffix(q, s.suffix); ok {
			value, err := strconv.ParseFloat(number, 64)
			if err != nil {
				return 0
			}
			return value * s.factor
		}
	}
	value, err := strconv.ParseFloat(q, 64)
	if err != nil {
		return 0
	}
	return value
}

// unix is 0 for a time the API left out
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
