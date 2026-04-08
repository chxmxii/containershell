package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/containershell/containershell/pkg/runtime"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const debugImage = "busybox:latest"

// DebugContainerStrategy injects a debug container that shares the target's namespaces.
type DebugContainerStrategy struct{}

func (s *DebugContainerStrategy) Name() string { return "debug container injection" }

func (s *DebugContainerStrategy) Try(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error {
	// Try K8s ephemeral containers first if we have pod info
	if container.PodName != "" && container.Namespace != "" {
		err := s.tryK8sEphemeral(ctx, container, verbose)
		if err == nil {
			return nil
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "  K8s ephemeral container failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Falling back to CRI-level debug container...\n")
		}
	}

	// Fall back to CRI-level: use nsenter from a privileged helper
	return s.tryCRIDebug(ctx, rt, container, verbose)
}

func (s *DebugContainerStrategy) tryK8sEphemeral(ctx context.Context, container *runtime.ContainerInfo, verbose bool) error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("no kubeconfig available: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create K8s client: %w", err)
	}

	debugName := fmt.Sprintf("containershell-debug-%d", time.Now().Unix())

	pod, err := clientset.CoreV1().Pods(container.Namespace).Get(ctx, container.PodName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %s/%s: %w", container.Namespace, container.PodName, err)
	}

	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    debugName,
			Image:   debugImage,
			Command: []string{"/bin/sh"},
			Stdin:   true,
			TTY:     true,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"SYS_PTRACE", "NET_RAW", "NET_ADMIN"},
				},
			},
		},
		TargetContainerName: container.Name,
	}

	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
	_, err = clientset.CoreV1().Pods(container.Namespace).UpdateEphemeralContainers(
		ctx, container.PodName, pod, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ephemeral container: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Created ephemeral container %s targeting %s\n", debugName, container.Name)
	}

	// Wait for the ephemeral container to be running
	for i := 0; i < 30; i++ {
		pod, err = clientset.CoreV1().Pods(container.Namespace).Get(ctx, container.PodName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to poll pod status: %w", err)
		}
		for _, cs := range pod.Status.EphemeralContainerStatuses {
			if cs.Name == debugName && cs.State.Running != nil {
				return kubectlAttach(container.Namespace, container.PodName, debugName)
			}
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("ephemeral container %s did not start within 30s", debugName)
}

func kubectlAttach(namespace, pod, container string) error {
	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found: %w", err)
	}

	cmd := exec.Command(kubectl, "attach", "-it",
		"-n", namespace,
		pod,
		"-c", container,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *DebugContainerStrategy) tryCRIDebug(ctx context.Context, rt runtime.Runtime, container *runtime.ContainerInfo, verbose bool) error {
	pid, err := rt.ContainerPid(ctx, container.ID)
	if err != nil {
		return fmt.Errorf("cannot determine container PID for CRI debug: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "  Target container PID: %d\n", pid)
		fmt.Fprintf(os.Stderr, "  Launching debug container with shared namespaces via nsenter...\n")
	}

	for _, crt := range []string{"podman", "docker"} {
		runtimePath, err := exec.LookPath(crt)
		if err != nil {
			continue
		}

		cmd := exec.Command(runtimePath, "run", "-it", "--rm",
			"--pid=host",
			fmt.Sprintf("--network=container:%s", container.ID),
			"--privileged",
			debugImage,
			"nsenter", fmt.Sprintf("--target=%d", pid),
			"--mount", "--uts", "--ipc", "--net", "--pid",
			"--", "/bin/sh",
		)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("CRI-level debug container injection failed: no container runtime (docker/podman) available")
}
