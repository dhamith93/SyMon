package monitor

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// labels the kubelet puts on every container it starts through CRI
const (
	podNamespaceLabel  = "io.kubernetes.pod.namespace"
	podNameLabel       = "io.kubernetes.pod.name"
	containerNameLabel = "io.kubernetes.container.name"
)

// criTimeout bounds a call on the local socket, anything slower is a wedged
// runtime
const criTimeout = 2 * time.Second

// podContainer is what CRI says about one container of a pod
type podContainer struct {
	namespace string
	pod       string
	name      string
	image     string
	// state is running, exited, created or unknown
	state string
	// sandbox is a pod's pause container, which holds its namespaces
	sandbox bool
}

// criClient asks the container runtime's CRI socket (containerd or CRI-O)
// which pod each container belongs to. It needs no Kubernetes credentials,
// only access to the socket.
type criClient struct {
	client runtimeapi.RuntimeServiceClient
}

// newCRIClient does not connect yet, that happens on the first call
func newCRIClient(socket string) (*criClient, error) {
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &criClient{client: runtimeapi.NewRuntimeServiceClient(conn)}, nil
}

// podContainers returns the pod containers by ID, and the pod sandboxes,
// whose ID is their pause container's
func (c *criClient) podContainers(ctx context.Context) (map[string]podContainer, error) {
	ctx, cancel := context.WithTimeout(ctx, criTimeout)
	defer cancel()
	resp, err := c.client.ListContainers(ctx, &runtimeapi.ListContainersRequest{})
	if err != nil {
		return nil, err
	}
	containers := make(map[string]podContainer, len(resp.Containers))
	for _, container := range resp.Containers {
		labels := container.GetLabels()
		if labels[podNameLabel] == "" {
			continue
		}
		name := labels[containerNameLabel]
		if name == "" {
			name = container.GetMetadata().GetName()
		}
		// the kubelet passes the image ID and keeps what the pod asked for
		// in UserSpecifiedImage
		image := container.GetImage().GetUserSpecifiedImage()
		if image == "" {
			image = container.GetImage().GetImage()
		}
		containers[container.GetId()] = podContainer{
			namespace: labels[podNamespaceLabel],
			pod:       labels[podNameLabel],
			name:      name,
			image:     image,
			state:     strings.ToLower(strings.TrimPrefix(container.GetState().String(), "CONTAINER_")),
		}
	}
	sandboxes, err := c.client.ListPodSandbox(ctx, &runtimeapi.ListPodSandboxRequest{})
	if err != nil {
		return nil, err
	}
	for _, sandbox := range sandboxes.Items {
		containers[sandbox.GetId()] = podContainer{sandbox: true}
	}
	return containers, nil
}

// namePodContainer names a pod's container from what CRI said, and reports
// false for a pause container. Static pods, like the control plane's, have a
// UID systats does not find in the cgroup path, so CRI is asked about every
// container. A pod's container CRI does not know yet started after the list
// and waits for the next round. Without an answer from CRI every container
// is kept as it is.
func namePodContainer(container *Container, pods map[string]podContainer) bool {
	if pods == nil {
		return true
	}
	pod, ok := pods[container.ID]
	if !ok {
		return container.PodUID == ""
	}
	if pod.sandbox {
		return false
	}
	pod.apply(container)
	return true
}

// apply names a container namespace/pod/container, which is also the name
// its metrics are stored under
func (p podContainer) apply(container *Container) {
	container.Namespace = p.namespace
	container.Pod = p.pod
	container.Name = p.namespace + "/" + p.pod + "/" + p.name
	if p.image != "" {
		container.Image = p.image
	}
	container.State = p.state
	container.MetadataAvailable = true
}
