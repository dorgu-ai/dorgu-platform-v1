/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command devcluster runs a real Kubernetes API server, installs the dorgu.io
// CRDs, fills it with a brownfield-shaped cluster, and writes a kubeconfig.
//
// It exists so the dashboard can be verified end to end without a cloud cluster
// and without pointing anything at a real one. The API server is the genuine
// article from envtest, so watches, server-side apply and field managers all
// behave as they do in production; there are no nodes and no kubelet, so Pod
// status is whatever this program writes, which is exactly what a fixture wants.
//
// Usage:
//
//	go run ./hack/devcluster                  # start, seed, print the kubeconfig path
//	go run ./hack/devcluster -break checkout  # also push an app into CrashLoopBackOff
//
// Then, in another shell:
//
//	KUBECONFIG=<printed path> go run ./cmd/dorgu-dashboard
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

func main() {
	crdDir := flag.String("crds", defaultCRDDir(),
		"directory holding the dorgu.io CRD manifests")
	noCRDs := flag.Bool("no-crds", false,
		"skip installing the dorgu.io CRDs, to exercise the operator-not-installed screens")
	breakApp := flag.String("break", "",
		"put this app's pod into CrashLoopBackOff and raise an incident for it")
	flag.Parse()

	if err := run(*crdDir, *noCRDs, *breakApp); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(crdDir string, noCRDs bool, breakApp string) error {
	env := &envtest.Environment{}
	if !noCRDs {
		env.CRDDirectoryPaths = []string{crdDir}
		env.ErrorIfCRDPathMissing = true
	}

	fmt.Println("starting a real API server (envtest)...")
	cfg, err := env.Start()
	if err != nil {
		return fmt.Errorf("starting the API server: %w (is KUBEBUILDER_ASSETS set?)", err)
	}
	defer func() {
		fmt.Println("stopping the API server")
		_ = env.Stop()
	}()

	kubeconfigPath, err := writeKubeconfig(cfg.Host, env.WebhookInstallOptions.LocalServingCAData, cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := seed(ctx, cfg, noCRDs, breakApp); err != nil {
		return err
	}

	fmt.Printf("\nkubeconfig written to:\n  %s\n\n", kubeconfigPath)
	fmt.Printf("run the dashboard against it:\n  KUBECONFIG=%s go run ./cmd/dorgu-dashboard\n\n", kubeconfigPath)
	fmt.Println("the API server stays up until this process is interrupted. Ctrl-C to stop.")

	<-ctx.Done()
	return nil
}

// seed builds a cluster shaped like a real brownfield one: a Helm-owned app with
// a persona whose name does not match its Deployment, an unmanaged app, an
// unmonitored Deployment nobody has imported, and a system namespace that should
// stay out of the way.
func seed(ctx context.Context, cfg *runtimeConfig, noCRDs bool, breakApp string) error {
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("building the typed client: %w", err)
	}

	crScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(crScheme); err != nil {
		return err
	}
	if err := dorguv1.AddToScheme(crScheme); err != nil {
		return err
	}
	ctrlClient, err := client.New(cfg, client.Options{Scheme: crScheme})
	if err != nil {
		return fmt.Errorf("building the controller-runtime client: %w", err)
	}

	for _, ns := range []string{"apps", "legacy", "kube-system"} {
		_, err := typed.CoreV1().Namespaces().Create(ctx,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
			metav1.CreateOptions{})
		// kube-system exists on a fresh API server already, which is the point
		// of including it: it is the namespace the unmonitored scan must skip.
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating namespace %s: %w", ns, err)
		}
	}

	for _, spec := range fixtureWorkloads() {
		if err := createWorkload(ctx, typed, spec); err != nil {
			return err
		}
	}

	if noCRDs {
		fmt.Println("seeded workloads; dorgu.io CRDs deliberately not installed")
		return nil
	}

	for _, persona := range fixturePersonas() {
		// Status is a subresource, so it needs its own write, and Create
		// decodes the server's reply back into this object with the status
		// stripped. Keeping a copy first is the difference between a fixture
		// with an operator verdict on it and one silently without.
		desired := persona.Status
		if err := ctrlClient.Create(ctx, persona); err != nil {
			return fmt.Errorf("creating persona %s: %w", persona.Name, err)
		}
		persona.Status = desired
		if err := ctrlClient.Status().Update(ctx, persona); err != nil {
			return fmt.Errorf("setting status on persona %s: %w", persona.Name, err)
		}
	}

	for _, incident := range fixtureIncidents() {
		desired := incident.Status
		if err := ctrlClient.Create(ctx, incident); err != nil {
			return fmt.Errorf("creating incident %s: %w", incident.Name, err)
		}
		incident.Status = desired
		if err := ctrlClient.Status().Update(ctx, incident); err != nil {
			return fmt.Errorf("setting status on incident %s: %w", incident.Name, err)
		}
	}

	if breakApp != "" {
		if err := breakWorkload(ctx, typed, ctrlClient, breakApp); err != nil {
			return err
		}
	}

	fmt.Println("seeded: 4 Deployments, 3 personas, 3 incidents")
	return nil
}

func defaultCRDDir() string {
	// The operator checkout is a sibling of this one in the standard layout.
	return filepath.Join("..", "dorgu-operator", "config", "crd", "bases")
}

func writeKubeconfig(host string, _ []byte, cfg *runtimeConfig) (string, error) {
	config := clientcmdapi.NewConfig()
	config.Clusters["devcluster"] = &clientcmdapi.Cluster{
		Server:                   host,
		CertificateAuthorityData: cfg.CAData,
	}
	config.AuthInfos["devcluster"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	config.Contexts["devcluster"] = &clientcmdapi.Context{
		Cluster:  "devcluster",
		AuthInfo: "devcluster",
	}
	config.CurrentContext = "devcluster"

	path := filepath.Join(os.TempDir(), "dorgu-devcluster.kubeconfig")
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		return "", fmt.Errorf("writing kubeconfig: %w", err)
	}
	return path, nil
}

// breakWorkload drives one app into a crash loop and files an incident against
// it, which is the state the Apps and Incidents views exist to show.
func breakWorkload(
	ctx context.Context,
	typed kubernetes.Interface,
	ctrlClient client.Client,
	app string,
) error {
	pods, err := typed.CoreV1().Pods("apps").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + app,
	})
	if err != nil {
		return fmt.Errorf("listing pods for %s: %w", app, err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods labelled app.kubernetes.io/name=%s", app)
	}

	pod := &pods.Items[0]
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "podinfo",
		RestartCount: 7,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CrashLoopBackOff",
			Message: "back-off 5m0s restarting failed container=podinfo",
		}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Reason:   "OOMKilled",
			ExitCode: 137,
		}},
	}}
	if _, err := typed.CoreV1().Pods("apps").UpdateStatus(ctx, pod, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("setting pod status: %w", err)
	}

	now := metav1.NewTime(time.Now())
	incident := &dorguv1.IncidentMemory{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: app + "-crashloop"},
		Spec: dorguv1.IncidentMemorySpec{
			PersonaRef: dorguv1.PersonaReference{
				Kind: "ApplicationPersona", Name: app, Namespace: "apps",
			},
			Attribution: "persona",
			Category:    "health",
			Severity:    "critical",
			Detection: dorguv1.DetectionInfo{
				Signal:    "CrashLoopBackOff",
				Source:    "pod-failure-detector",
				FirstSeen: now,
				LastSeen:  now,
				AffectedResources: []dorguv1.ResourceReference{
					{Kind: "Pod", Name: pod.Name, Namespace: "apps", Role: "affected"},
				},
			},
		},
	}
	if err := ctrlClient.Create(ctx, incident); err != nil {
		return fmt.Errorf("creating the crash-loop incident: %w", err)
	}
	incident.Status = dorguv1.IncidentMemoryStatus{Phase: "Detected", OccurrenceCount: 7}
	if err := ctrlClient.Status().Update(ctx, incident); err != nil {
		return fmt.Errorf("setting incident status: %w", err)
	}

	fmt.Printf("broke %s: pod in CrashLoopBackOff, critical incident filed\n", app)
	return nil
}

func quantity(value string) resource.Quantity {
	return resource.MustParse(value)
}
