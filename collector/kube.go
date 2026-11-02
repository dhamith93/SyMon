package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/kube"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/store"
)

// kubeTarget is a node or workload a kubernetes rule checks, with its value
// and what its pods say is wrong
type kubeTarget struct {
	target string
	value  float64
	reason string
}

// evaluateCluster checks a kubernetes rule against every node or workload
// in its cluster's newest state. A state too old to trust is no data, so a
// reader that stopped does not open or resolve anything. An open alert whose
// node or workload is gone from the cluster resolves.
func (e *evaluator) evaluateCluster(ctx context.Context, rule *alerts.AlertConfig) error {
	cluster, err := e.store.Cluster(ctx, rule.Cluster)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if cluster.Stale(e.now()) {
		return nil
	}
	at := time.Unix(cluster.State.Time, 0)
	var restarts map[string]int
	if rule.MetricName == monitor.KUBE_RESTARTS {
		restarts, err = e.store.RestartsSince(ctx, rule.Cluster, at.Add(-time.Hour))
		if err != nil {
			return err
		}
	}

	targets := kubeTargets(rule, cluster.State, restarts)
	for _, t := range targets {
		e.reasons[rule.Name+"|"+t.target] = t.reason
		if err := e.check(ctx, rule, "", t.target, t.value, at); err != nil {
			return err
		}
	}
	return e.resolveGone(ctx, rule, targets, at)
}

// kubeTargets are what a rule checks in a state. Pending and restart rules
// check every workload and the pods that belong to none.
func kubeTargets(rule *alerts.AlertConfig, state kube.ClusterState, restarts map[string]int) []kubeTarget {
	var targets []kubeTarget
	if rule.MetricName == monitor.KUBE_NODE {
		for _, n := range state.Nodes {
			value := 0.0
			if !n.Ready {
				value = 1
			}
			targets = append(targets, kubeTarget{target: n.Name, value: value, reason: strings.Join(n.Pressure, ", ")})
		}
		return targets
	}

	inNamespace := func(namespace string) bool { return rule.Namespace == "" || rule.Namespace == namespace }
	pods := map[string][]kube.Pod{}
	var keys []string
	add := func(key string) {
		if _, ok := pods[key]; !ok {
			pods[key] = nil
			keys = append(keys, key)
		}
	}
	for _, w := range state.Workloads {
		if inNamespace(w.Namespace) {
			add(w.Key())
		}
	}
	if rule.MetricName != monitor.KUBE_WORKLOAD {
		for _, p := range state.Pods {
			if inNamespace(p.Namespace) {
				add(p.WorkloadKey())
			}
		}
	}
	for _, p := range state.Pods {
		if list, ok := pods[p.WorkloadKey()]; ok {
			pods[p.WorkloadKey()] = append(list, p)
		}
	}

	switch rule.MetricName {
	case monitor.KUBE_WORKLOAD:
		for _, w := range state.Workloads {
			if inNamespace(w.Namespace) {
				targets = append(targets, kubeTarget{target: w.Key(), value: float64(w.Missing()), reason: podReason(pods[w.Key()], false)})
			}
		}
	case monitor.KUBE_PENDING:
		for _, key := range keys {
			var pending []kube.Pod
			for _, p := range pods[key] {
				if p.Phase == "Pending" {
					pending = append(pending, p)
				}
			}
			targets = append(targets, kubeTarget{target: key, value: float64(len(pending)), reason: podReason(pending, false)})
		}
	case monitor.KUBE_RESTARTS:
		for _, key := range keys {
			targets = append(targets, kubeTarget{target: key, value: float64(restarts[key]), reason: podReason(pods[key], true)})
		}
	}
	return targets
}

// podReason is the first thing the pods say is wrong. With lastRun it falls
// back to why a container's run before this one ended, like OOMKilled.
func podReason(pods []kube.Pod, lastRun bool) string {
	for _, p := range pods {
		if p.Reason != "" {
			return p.Reason
		}
	}
	if lastRun {
		for _, p := range pods {
			for _, c := range p.Containers {
				if c.LastReason != "" {
					return c.LastReason
				}
			}
		}
	}
	return ""
}

// resolveGone resolves a rule's open alerts whose node or workload is not
// in the cluster any more, like a deleted deployment, and forgets what the
// evaluator kept about them
func (e *evaluator) resolveGone(ctx context.Context, rule *alerts.AlertConfig, targets []kubeTarget, at time.Time) error {
	present := map[string]bool{}
	for _, t := range targets {
		present[t.target] = true
	}
	open, err := e.store.Alerts(ctx, store.AlertFilter{Rule: rule.Name, OpenOnly: true})
	if err != nil {
		return err
	}
	for _, a := range open {
		if a.Metric != rule.MetricName || present[a.Target] {
			continue
		}
		if err := e.store.ResolveAlert(ctx, a.ID, 0, at); err != nil {
			return err
		}
		n := e.message(rule, "", a.Target, a.ID, notify.Resolved, alertstatus.Normal, 0, at)
		n.subject, n.body = kubeGoneText(rule, a.Target, at)
		e.notify(ctx, n)
	}

	prefix := alertKey("", rule, "")
	for key := range e.pending {
		if target, ok := strings.CutPrefix(key, prefix); ok && !present[target] {
			delete(e.pending, key)
		}
	}
	for key := range e.checked {
		if target, ok := strings.CutPrefix(key, prefix); ok && !present[target] {
			delete(e.checked, key)
		}
	}
	for key := range e.reasons {
		if target, ok := strings.CutPrefix(key, rule.Name+"|"); ok && !present[target] {
			delete(e.reasons, key)
		}
	}
	return nil
}

// kubeProblem and kubeFine say what is wrong with a target and that it is
// fine again
func kubeProblem(metric string, target string, value float64) string {
	n := int(value)
	switch metric {
	case monitor.KUBE_NODE:
		return "node " + target + " is not ready"
	case monitor.KUBE_WORKLOAD:
		return target + " is missing " + plural(n, "replica")
	case monitor.KUBE_PENDING:
		return plural(n, "pod") + " of " + target + " stuck pending"
	}
	return target + " restarted " + plural(n, "time") + " in the last hour"
}

func kubeFine(metric string, target string) string {
	switch metric {
	case monitor.KUBE_NODE:
		return "node " + target + " is ready again"
	case monitor.KUBE_WORKLOAD:
		return target + " has all its replicas again"
	case monitor.KUBE_PENDING:
		return target + " has no pods pending"
	}
	return target + " stopped restarting"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// kubeText fills a kubernetes rule's template. reason is what the pods say,
// like CrashLoopBackOff, or a node's pressure.
func kubeText(rule *alerts.AlertConfig, target string, reason string, status alertstatus.StatusType, value float64, at time.Time) (string, string) {
	if status == alertstatus.Normal {
		fine := kubeFine(rule.MetricName, target)
		return "[Resolved] " + rule.Name + ": " + fine + " in " + rule.Cluster,
			fine + " in " + rule.Cluster + " at " + at.UTC().Format(time.RFC1123) + "."
	}
	level := "[Critical] "
	if status == alertstatus.Warning {
		level = "[Warning] "
	}
	problem := kubeProblem(rule.MetricName, target, value)
	subject := level + rule.Name + ": " + problem + " in " + rule.Cluster
	replacer := strings.NewReplacer(
		"{subject}", subject,
		"{cluster}", rule.Cluster,
		"{target}", target,
		"{metricName}", rule.MetricName,
		"{value}", fmt.Sprintf("%.0f", value),
		"{reason}", reason,
		"{timestamp}", at.UTC().String(),
		"{desc}", rule.Description,
		"{triggerInterval}", strconv.Itoa(rule.TriggerIntveral),
	)
	body := replacer.Replace(rule.Template)
	if strings.TrimSpace(body) == "" {
		body = problem + " in " + rule.Cluster + "."
		if reason != "" {
			body += " The pods say " + reason + "."
			if rule.MetricName == monitor.KUBE_NODE {
				body = problem + " in " + rule.Cluster + ". It reports " + reason + "."
			}
		}
	}
	return subject, body
}

func kubeGoneText(rule *alerts.AlertConfig, target string, at time.Time) (string, string) {
	what := target
	if rule.MetricName == monitor.KUBE_NODE {
		what = "node " + target
	}
	return "[Resolved] " + rule.Name + ": " + what + " is gone from " + rule.Cluster,
		what + " was removed from " + rule.Cluster + " by " + at.UTC().Format(time.RFC1123) + "."
}
