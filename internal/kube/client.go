package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// listLimit is how many objects one request asks for, the rest comes with
// the continue token
const listLimit = 500

// Client reads the cluster with a service account token. It needs get and
// list on nodes, pods and events, and on apps deployments, statefulsets
// and daemonsets (docs/kubernetes/symon-reader.yaml).
type Client struct {
	base      string
	tokenPath string
	http      *http.Client
}

// NewClient checks the settings and reads the CA. The token is read again
// for every round, so a rotated token keeps working.
func NewClient(apiURL string, caPath string, tokenPath string) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("the API URL %q is not an https:// URL", apiURL)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPath != "" {
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read the cluster CA: %w", err)
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificate in %s", caPath)
		}
	}
	return &Client{
		base:      base.String(),
		tokenPath: tokenPath,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment},
		},
	}, nil
}

// Read lists the cluster. When the API cannot be read the state carries the
// error and nothing else, so the dashboard can say why.
func (c *Client) Read(ctx context.Context, cluster string, now time.Time) ClusterState {
	state, err := c.read(ctx, now)
	if err != nil {
		return ClusterState{Cluster: cluster, Time: now.Unix(), Error: err.Error()}
	}
	state.Cluster = cluster
	state.Time = now.Unix()
	return state
}

func (c *Client) read(ctx context.Context, now time.Time) (ClusterState, error) {
	raw, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return ClusterState{}, fmt.Errorf("cannot read the token: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return ClusterState{}, fmt.Errorf("the token file %s is empty", c.tokenPath)
	}

	nodes, err := list[apiNode](ctx, c, token, "/api/v1/nodes", "")
	if err != nil {
		return ClusterState{}, err
	}
	pods, err := list[apiPod](ctx, c, token, "/api/v1/pods", "")
	if err != nil {
		return ClusterState{}, err
	}
	deployments, err := list[apiReplicated](ctx, c, token, "/apis/apps/v1/deployments", "")
	if err != nil {
		return ClusterState{}, err
	}
	statefulSets, err := list[apiReplicated](ctx, c, token, "/apis/apps/v1/statefulsets", "")
	if err != nil {
		return ClusterState{}, err
	}
	daemonSets, err := list[apiDaemonSet](ctx, c, token, "/apis/apps/v1/daemonsets", "")
	if err != nil {
		return ClusterState{}, err
	}
	events, err := list[apiEvent](ctx, c, token, "/api/v1/events", "type=Warning")
	if err != nil {
		return ClusterState{}, err
	}

	state := ClusterState{
		Nodes:     make([]Node, 0, len(nodes)),
		Workloads: make([]Workload, 0, len(deployments)+len(statefulSets)+len(daemonSets)),
		Pods:      make([]Pod, 0, len(pods)),
	}
	for _, n := range nodes {
		state.Nodes = append(state.Nodes, n.node())
	}
	for _, d := range deployments {
		state.Workloads = append(state.Workloads, d.workload("deployment"))
	}
	for _, s := range statefulSets {
		state.Workloads = append(state.Workloads, s.workload("statefulset"))
	}
	for _, d := range daemonSets {
		state.Workloads = append(state.Workloads, d.workload())
	}
	for _, p := range pods {
		state.Pods = append(state.Pods, p.pod())
	}
	state.Events = warningEvents(events, now)
	return state, nil
}

// list reads every page of a list. fieldSelector may be empty.
func list[T any](ctx context.Context, c *Client, token string, path string, fieldSelector string) ([]T, error) {
	var items []T
	next := ""
	for {
		query := url.Values{"limit": {fmt.Sprint(listLimit)}}
		if next != "" {
			query.Set("continue", next)
		}
		if fieldSelector != "" {
			query.Set("fieldSelector", fieldSelector)
		}
		var page struct {
			Metadata struct {
				Continue string `json:"continue"`
			} `json:"metadata"`
			Items []T `json:"items"`
		}
		if err := c.get(ctx, token, path+"?"+query.Encode(), &page); err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.Metadata.Continue == "" {
			return items, nil
		}
		next = page.Metadata.Continue
	}
}

func (c *Client) get(ctx context.Context, token string, pathAndQuery string, into any) error {
	path, _, _ := strings.Cut(pathAndQuery, "?")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathAndQuery, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error repeats the URL, the cause is enough
		var failed *url.Error
		if errors.As(err, &failed) {
			err = failed.Err
		}
		return fmt.Errorf("cannot reach the API for %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the API answered %s for %s%s", resp.Status, path, statusMessage(resp.Body))
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("cannot decode %s: %w", path, err)
	}
	return nil
}

// statusMessage is the message of a Kubernetes Status reply, which says
// which permission is missing
func statusMessage(body io.Reader) string {
	var status struct {
		Message string `json:"message"`
	}
	if json.NewDecoder(io.LimitReader(body, 64<<10)).Decode(&status) != nil || status.Message == "" {
		return ""
	}
	message := status.Message
	if len(message) > 200 {
		message = message[:200] + "…"
	}
	return ": " + message
}
