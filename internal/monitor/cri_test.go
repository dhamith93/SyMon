package monitor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/systats"
	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type fakeRuntime struct {
	runtimeapi.UnimplementedRuntimeServiceServer
	containers []*runtimeapi.Container
	sandboxes  []*runtimeapi.PodSandbox
}

func (f *fakeRuntime) ListContainers(context.Context, *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	return &runtimeapi.ListContainersResponse{Containers: f.containers}, nil
}

func (f *fakeRuntime) ListPodSandbox(context.Context, *runtimeapi.ListPodSandboxRequest) (*runtimeapi.ListPodSandboxResponse, error) {
	return &runtimeapi.ListPodSandboxResponse{Items: f.sandboxes}, nil
}

// fakeCRISocket serves a runtime on a unix socket and returns its path
func fakeCRISocket(t *testing.T, runtime *fakeRuntime) string {
	t.Helper()
	// t.TempDir is too long for a unix socket path on macOS
	dir, err := os.MkdirTemp("", "cri")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "cri.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	runtimeapi.RegisterRuntimeServiceServer(server, runtime)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	return socket
}

func podLabels(namespace, pod, container string) map[string]string {
	return map[string]string{podNamespaceLabel: namespace, podNameLabel: pod, containerNameLabel: container}
}

func TestPodContainersFromCRI(t *testing.T) {
	socket := fakeCRISocket(t, &fakeRuntime{
		containers: []*runtimeapi.Container{
			{
				Id:     "aaa",
				Labels: podLabels("media", "jellyfin-7d9f8b6c5-x2k4p", "jellyfin"),
				Image:  &runtimeapi.ImageSpec{Image: "sha256:1234", UserSpecifiedImage: "jellyfin/jellyfin:10.10"},
				State:  runtimeapi.ContainerState_CONTAINER_RUNNING,
			},
			{
				Id:       "bbb",
				Metadata: &runtimeapi.ContainerMetadata{Name: "sidecar"},
				Labels:   map[string]string{podNamespaceLabel: "kube-system", podNameLabel: "coredns-1"},
				Image:    &runtimeapi.ImageSpec{Image: "registry.k8s.io/coredns:1.11"},
				State:    runtimeapi.ContainerState_CONTAINER_EXITED,
			},
			// started outside the kubelet, so not a pod's
			{Id: "ccc", Labels: map[string]string{}},
		},
		sandboxes: []*runtimeapi.PodSandbox{{Id: "ppp"}},
	})
	cri, err := newCRIClient(socket)
	if err != nil {
		t.Fatal(err)
	}
	pods, err := cri.podContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]podContainer{
		"aaa": {namespace: "media", pod: "jellyfin-7d9f8b6c5-x2k4p", name: "jellyfin", image: "jellyfin/jellyfin:10.10", state: "running"},
		"bbb": {namespace: "kube-system", pod: "coredns-1", name: "sidecar", image: "registry.k8s.io/coredns:1.11", state: "exited"},
		"ppp": {sandbox: true},
	}
	if len(pods) != len(want) {
		t.Fatalf("got %v, want %v", pods, want)
	}
	for id, w := range want {
		if pods[id] != w {
			t.Errorf("%s: got %+v, want %+v", id, pods[id], w)
		}
	}
}

func TestNamePodContainer(t *testing.T) {
	pods := map[string]podContainer{
		"aaa": {namespace: "media", pod: "web-1", name: "nginx", image: "nginx:1.27", state: "running"},
		"etc": {namespace: "kube-system", pod: "etcd-cp1", name: "etcd", image: "registry.k8s.io/etcd:3.6", state: "running"},
		"ppp": {sandbox: true},
	}

	named := Container{ID: "aaa", ShortID: "aaa", PodUID: "uid-1"}
	if !namePodContainer(&named, pods) {
		t.Fatal("a listed container was dropped")
	}
	if named.Name != "media/web-1/nginx" || named.Namespace != "media" || named.Pod != "web-1" || named.Image != "nginx:1.27" || named.State != "running" || !named.MetadataAvailable {
		t.Errorf("not named from CRI: %+v", named)
	}

	if pause := (Container{ID: "ppp", PodUID: "uid-1"}); namePodContainer(&pause, pods) {
		t.Error("a pause container was kept")
	}
	// a static pod's UID is a hash systats does not read from the cgroup path
	if static := (Container{ID: "etc"}); !namePodContainer(&static, pods) || static.Name != "kube-system/etcd-cp1/etcd" {
		t.Errorf("a static pod's container was not named: %+v", static)
	}
	if staticPause := (Container{ID: "ppp"}); namePodContainer(&staticPause, pods) {
		t.Error("a static pod's pause container was kept")
	}
	if late := (Container{ID: "bbb", PodUID: "uid-2"}); namePodContainer(&late, pods) {
		t.Error("kept a pod's container CRI did not list, it would be stored under its ID")
	}
	if docker := (Container{ID: "ccc", Name: "db"}); !namePodContainer(&docker, pods) || docker.Name != "db" {
		t.Errorf("a container outside kubernetes changed: %+v", docker)
	}
	// without an answer from CRI nothing is dropped
	if unknown := (Container{ID: "bbb", PodUID: "uid-1"}); !namePodContainer(&unknown, nil) {
		t.Error("dropped a container while CRI was not reachable")
	}
}

func TestPodContainersOnlyWithPods(t *testing.T) {
	c := NewCollector(&config.Agent{CRISocket: filepath.Join(t.TempDir(), "missing.sock")})
	if pods := c.podContainers(context.Background(), []systats.Container{{ID: "a"}}); pods != nil || c.cri != nil {
		t.Errorf("asked CRI on a host without pods: %v", pods)
	}
	if pods := c.podContainers(context.Background(), []systats.Container{{ID: "a", PodUID: "uid"}}); pods != nil || !c.criFailed {
		t.Errorf("expected a missing socket to fail once, got %v", pods)
	}
}
