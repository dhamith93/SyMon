package kube

// Restart is a container that restarted between two states
type Restart struct {
	Namespace string
	// Workload is kind/name, like Pod.Workload
	Workload  string
	Pod       string
	Container string
	Count     int
	// Reason is why the run before ended, like Error or OOMKilled
	Reason string
}

// Restarts compares two states of a cluster. A pod first seen in next only
// counts when it was created after prev, since restarts from before the
// reader saw it are history. A count that goes down (the kubelet lost its
// state) counts from zero. There are none for a cluster's first state.
func Restarts(prev ClusterState, next ClusterState) []Restart {
	if prev.Time == 0 {
		// the first state, nothing to compare with
		return nil
	}
	before := map[string]int{}
	known := map[string]bool{}
	for _, pod := range prev.Pods {
		known[pod.UID] = true
		for _, c := range pod.Containers {
			before[pod.UID+"/"+c.Name] = c.Restarts
		}
	}

	var restarts []Restart
	for _, pod := range next.Pods {
		if !known[pod.UID] && pod.Created <= prev.Time {
			continue
		}
		for _, c := range pod.Containers {
			count := c.Restarts
			if old, ok := before[pod.UID+"/"+c.Name]; ok && old <= count {
				count -= old
			}
			if count <= 0 {
				continue
			}
			restarts = append(restarts, Restart{
				Namespace: pod.Namespace,
				Workload:  pod.Workload,
				Pod:       pod.Name,
				Container: c.Name,
				Count:     count,
				Reason:    c.LastReason,
			})
		}
	}
	return restarts
}
