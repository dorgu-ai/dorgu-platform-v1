package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// Fixtures for the Remediations and Cluster views.
//
// envtest runs an API server and no controllers, so nodes and pod status are
// whatever this file writes. That is what makes it useful here: the shapes below
// are the ones that were expensive to get right, and each is a state a real
// cluster produced.

// ---------------------------------------------------------------------------
// Nodes and the pods that claim from them
// ---------------------------------------------------------------------------

// fixtureNodes are two Ready arm64 nodes, one of them cordoned.
//
// arm64 because that is what an Apple Silicon kind cluster and an AWS Graviton
// nodegroup report, and because every operator image before v0.11.1 was
// amd64-only, so the architecture column has a real reason to exist. The
// cordoned node is there because cordoned and NotReady are different facts and
// the view has to keep them apart: it is Ready, and closed on purpose.
func fixtureNodes() []*corev1.Node {
	joined := metav1.NewTime(time.Now().Add(-72 * time.Hour))

	return []*corev1.Node{
		nodeFixture("node-a", joined, "worker", false),
		nodeFixture("node-b", joined, "worker", true),
	}
}

func nodeFixture(name string, joined metav1.Time, role string, cordoned bool) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: joined,
			Labels: map[string]string{
				"node-role.kubernetes.io/" + role: "",
				"kubernetes.io/arch":              "arm64",
				"topology.kubernetes.io/zone":     "eu-west-1a",
			},
		},
		Spec: corev1.NodeSpec{Unschedulable: cordoned},
	}
	if cordoned {
		node.Spec.Taints = []corev1.Taint{{
			Key:    "node.kubernetes.io/unschedulable",
			Effect: corev1.TaintEffectNoSchedule,
		}}
	}
	return node
}

// nodeStatusFor is the status a kubelet would have written. envtest runs no
// kubelet, so the fixture supplies it, and allocatable is deliberately lower
// than capacity because that difference is what the view explains.
func nodeStatusFor(ready bool) corev1.NodeStatus {
	pool := func(cpu, memory, pods string) corev1.ResourceList {
		return corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memory),
			corev1.ResourcePods:   resource.MustParse(pods),
		}
	}

	condition := corev1.NodeCondition{
		Type:               corev1.NodeReady,
		Status:             corev1.ConditionTrue,
		Reason:             "KubeletReady",
		LastTransitionTime: metav1.NewTime(time.Now().Add(-72 * time.Hour)),
	}
	if !ready {
		condition.Status = corev1.ConditionFalse
		condition.Reason = "KubeletNotReady"
		condition.Message = "container runtime network not ready"
	}

	return corev1.NodeStatus{
		Capacity:    pool("2", "4Gi", "110"),
		Allocatable: pool("1930m", "3600Mi", "110"),
		Conditions:  []corev1.NodeCondition{condition},
		NodeInfo: corev1.NodeSystemInfo{
			KubeletVersion:          "v1.33.4",
			ContainerRuntimeVersion: "containerd://2.1.4",
			OperatingSystem:         "linux",
			Architecture:            "arm64",
		},
	}
}

// createNodes writes the nodes and their status.
func createNodes(ctx context.Context, typed kubernetes.Interface) error {
	for _, node := range fixtureNodes() {
		created, err := typed.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating node %s: %w", node.Name, err)
		}
		if apierrors.IsAlreadyExists(err) {
			continue
		}
		// Both nodes report Ready. The interesting distinction in this fixture is
		// node-b being Ready and cordoned: a cordoned node is healthy and
		// deliberately closed, and a view that folds that into NotReady reports a
		// maintenance window as a fault. A genuinely NotReady node is covered by
		// the unit tests, which can have as many nodes as they like.
		created.Status = nodeStatusFor(true)
		if _, err := typed.CoreV1().Nodes().UpdateStatus(ctx, created, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("setting status on node %s: %w", node.Name, err)
		}
	}
	return nil
}

// fixtureUnschedulablePods are the pods that made cluster health report 1689%.
//
// Three pods asking for far more CPU than the cluster owns, none of them
// accepted by a node. Before the fix their requests were summed against node
// allocatable, which is why the error had no upper bound. They are here so the
// exclusion and the warning that names them can be seen working rather than
// taken on trust.
func fixtureUnschedulablePods() []*corev1.Pod {
	var pods []*corev1.Pod
	for i := 1; i <= 3; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "apps",
				Name:      fmt.Sprintf("nightly-batch-%d", i),
				Labels:    map[string]string{"app": "nightly-batch"},
			},
			Spec: corev1.PodSpec{
				// No nodeName: nothing has accepted these, so they hold no
				// allocation anywhere.
				Containers: []corev1.Container{{
					Name:  "batch",
					Image: "example.internal/batch:1.0.0",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("21"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					}},
				}},
			},
		})
	}
	return pods
}

// createUnschedulablePods writes the pods and the Pending status a scheduler
// would have left on them.
func createUnschedulablePods(ctx context.Context, typed kubernetes.Interface) error {
	for _, pod := range fixtureUnschedulablePods() {
		created, err := typed.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("creating pod %s: %w", pod.Name, err)
		}
		created.Status = corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  "Unschedulable",
				Message: "0/2 nodes are available: insufficient cpu",
			}},
		}
		if _, err := typed.CoreV1().Pods(pod.Namespace).UpdateStatus(
			ctx, created, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("setting status on pod %s: %w", pod.Name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ClusterPersona
// ---------------------------------------------------------------------------

// fixtureClusterPersona is the operator's own description of the cluster.
//
// metrics-server is recorded as installed and healthy on purpose. envtest runs
// no metrics-server, so the dashboard's poller gets nothing, and the Cluster view
// contradicts the record rather than repeating it. That disagreement is the one
// add-on this process can check first-hand, and it is worth being able to see it
// working.
func fixtureClusterPersona() *dorguv1.ClusterPersona {
	healthy := true
	discovered := metav1.NewTime(time.Now().Add(-4 * time.Minute))

	return &dorguv1.ClusterPersona{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: dorguv1.ClusterPersonaSpec{
			Name:        "dorgu-devcluster",
			Environment: "development",
			Description: "A brownfield-shaped cluster, for verifying the dashboard without one.",
		},
		Status: dorguv1.ClusterPersonaStatus{
			Phase:             "Ready",
			LastDiscovery:     &discovered,
			KubernetesVersion: "v1.33.4",
			Platform:          "envtest",
			ApplicationCount:  3,
			Namespaces:        &dorguv1.NamespaceSummary{Total: 4, Active: 4, WithPersonas: 1},
			Addons: []dorguv1.AddonInfo{
				{
					Name: "metrics-server", Type: "monitoring", Namespace: "kube-system",
					Installed: true, Version: "v0.7.2", Healthy: &healthy,
				},
				{
					Name: "coredns", Type: "other", Namespace: "kube-system",
					Installed: true, Version: "v1.11.3", Healthy: &healthy,
				},
				{Name: "cluster-autoscaler", Type: "other", Installed: false},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// RemediationActions
// ---------------------------------------------------------------------------

// fixtureRemediations are the four plan shapes the view has to tell apart.
//
//   - fix-oom-checkout: the clean-room shape. An AI plan against a Helm-owned
//     workload, with a blast-radius guardrail that clamped 4Gi to 512Mi and an
//     absent-field rule that rejected a CPU limit the container does not set.
//     This is the one that proves a refused value appears in the verdict and in
//     no diff.
//   - fix-oom-reports: a rule-based plan against an unmanaged workload, which is
//     the only case Dorgu may patch the Deployment for. No guardrail ruled.
//   - restart-frontend: advisory only. Nothing in it can be applied, which is
//     what nine of nine AI plans looked like before operator v0.11.0.
//   - completed-checkout-cpu: already applied and verified, so the phase column
//     and the per-step status have something in them.
func fixtureRemediations() []*dorguv1.RemediationAction {
	observed := metav1.NewTime(time.Now().Add(-6 * time.Minute))
	applied := metav1.NewTime(time.Now().Add(-2 * time.Hour))

	helmWorkload := &dorguv1.WorkloadRef{
		Kind: "Deployment", Name: "checkout-podinfo", Namespace: "apps", Container: "podinfo",
		ManagedBy:       dorguv1.ManagedByHelm,
		ManagedByDetail: `Helm release "checkout" in namespace apps`,
		ObservedResources: &dorguv1.ObservedResources{
			Limits: &dorguv1.ResourceValues{Memory: "128Mi"},
		},
		ObservedImage: "ghcr.io/stefanprodan/podinfo:6.7.1",
		ObservedAt:    &observed,
	}

	return []*dorguv1.RemediationAction{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "fix-oom-checkout"},
			Spec: dorguv1.RemediationActionSpec{
				IncidentRef: dorguv1.IncidentReference{Name: "checkout-oomkilled", Namespace: "apps"},
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "checkout", Namespace: "apps",
				},
				TrustLevel: 2,
				Confidence: "0.88",
				PlanSource: dorguv1.PlanSourceAIAnthropic,
				PlanSummary: "The podinfo container is OOMKilled repeatedly at a 128Mi limit " +
					"while its working set grows during checkout traffic.",
				Explanation: "Raise the memory limit and watch the rollout.",
				WorkloadRef: helmWorkload,
				Action: dorguv1.RemediationActionDetail{
					Type:  dorguv1.ActionTypePersonaUpdate,
					Patch: rawPatch(`{"spec":{"resources":{"limits":{"memory":"256Mi"}}}}`),
				},
				Steps: []dorguv1.RemediationStep{
					{
						Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate,
						Risk: "low", AutoExecutable: true,
						// The operator writes a guarded step's description as its own
						// sentence followed by each safety message VERBATIM, which
						// is why the renderer takes them back out. Paraphrasing
						// them here would make the dedup silently do nothing, so
						// the two strings below are the safety messages copied
						// exactly.
						Description: "Set spec.resources.limits.memory to 256Mi on the " +
							"ApplicationPersona. " + clampedMemoryMessage + " " + rejectedCPUMessage,
						// The rationale is the model's, and it contradicts the
						// verdict below it. That contradiction is exactly what
						// the separate rendering exists to make visible.
						Rationale: "Raising the memory limit to 4Gi is well within a 2x ceiling and " +
							"gives the container ample headroom.",
						Patch:         rawPatch(`{"spec":{"resources":{"limits":{"memory":"256Mi"}}}}`),
						PrePatchState: rawPatch(`{"spec":{"resources":{"limits":{"memory":"128Mi"}}}}`),
						Safety: []dorguv1.StepSafety{
							{
								Rule:     dorguv1.SafetyRuleBlastRadius,
								Verdict:  dorguv1.SafetyVerdictClamped,
								Field:    "spec.resources.limits.memory",
								Baseline: "128Mi", Requested: "4Gi", Permitted: "256Mi",
								Ratio: "32.0x", MaxRatio: "2.0x",
								Message: clampedMemoryMessage,
							},
							{
								Rule:      dorguv1.SafetyRuleAbsentField,
								Verdict:   dorguv1.SafetyVerdictRejected,
								Field:     "spec.resources.limits.cpu",
								Requested: "500m",
								Message:   rejectedCPUMessage,
							},
						},
					},
					{
						Order: 2, ID: "watch-rollout", Type: dorguv1.StepTypeWorkloadApply,
						Risk:        "low",
						Description: "Set resources.limits.memory to 256Mi in your Helm values, then run your usual helm upgrade.",
						Command:     "kubectl rollout status deploy/checkout-podinfo -n apps",
					},
					{
						Order: 3, ID: "confirm-no-oom", Type: dorguv1.StepTypeManual,
						Risk:        "low",
						Description: "Confirm the container stops being OOMKilled.",
						// A write on a Helm-owned workload. The view withholds it
						// and says why, which is the guard being visible.
						Command: "kubectl patch deploy/checkout-podinfo -n apps --type merge -p {}",
					},
				},
				Approval: &dorguv1.ApprovalSpec{Required: true},
				Rollback: &dorguv1.RemediationRollbackSpec{
					Enabled:          true,
					HealthCheckAfter: &metav1.Duration{Duration: 5 * time.Minute},
					MaxRetries:       1,
				},
			},
			Status: dorguv1.RemediationActionStatus{Phase: "Pending"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "fix-oom-reports"},
			Spec: dorguv1.RemediationActionSpec{
				IncidentRef: dorguv1.IncidentReference{Name: "reports-oomkilled", Namespace: "apps"},
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "reports", Namespace: "apps",
				},
				TrustLevel:  2,
				Confidence:  "0.72",
				PlanSource:  dorguv1.PlanSourceRuleBased,
				PlanSummary: "The reports worker is OOMKilled at its 256Mi limit.",
				Explanation: "Raise the memory limit to 512Mi.",
				WorkloadRef: &dorguv1.WorkloadRef{
					Kind: "Deployment", Name: "reports", Namespace: "apps", Container: "reports",
					ManagedBy: dorguv1.ManagedByUnmanaged,
					ObservedResources: &dorguv1.ObservedResources{
						Limits: &dorguv1.ResourceValues{Memory: "256Mi"},
					},
					ObservedAt: &observed,
				},
				Action: dorguv1.RemediationActionDetail{
					Type:  dorguv1.ActionTypePersonaUpdate,
					Patch: rawPatch(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
				},
				Steps: []dorguv1.RemediationStep{{
					Order: 1, ID: "raise-memory", Type: dorguv1.StepTypePersonaUpdate,
					Risk: "low", AutoExecutable: true,
					Description:   "Set spec.resources.limits.memory to 512Mi on the ApplicationPersona.",
					Patch:         rawPatch(`{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}`),
					PrePatchState: rawPatch(`{"spec":{"resources":{"limits":{"memory":"256Mi"}}}}`),
				}},
				Approval: &dorguv1.ApprovalSpec{Required: true},
			},
			Status: dorguv1.RemediationActionStatus{Phase: "Pending"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "restart-frontend"},
			Spec: dorguv1.RemediationActionSpec{
				IncidentRef: dorguv1.IncidentReference{Name: "frontend-imagepull", Namespace: "apps"},
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "frontend", Namespace: "apps",
				},
				TrustLevel: 1,
				Confidence: "0.55",
				PlanSource: dorguv1.PlanSourceAIAnthropic,
				PlanSummary: "The image tag frontend:1.9.2 cannot be pulled. Dorgu has not read a " +
					"tag that exists, so it does not name one.",
				Explanation: "Correct the image reference where this workload's desired state lives.",
				WorkloadRef: &dorguv1.WorkloadRef{
					Kind: "Deployment", Name: "frontend-web", Namespace: "apps", Container: "web",
					ManagedBy:       dorguv1.ManagedByArgoCD,
					ManagedByDetail: `ArgoCD application "storefront"`,
					ObservedImage:   "example.internal/frontend:1.9.2",
					ObservedAt:      &observed,
				},
				// Advisory: a notification action with no appliable patch, which
				// is what nine of nine AI plans looked like before v0.11.0.
				Action: dorguv1.RemediationActionDetail{Type: dorguv1.ActionTypeNotification},
				Steps: []dorguv1.RemediationStep{
					{
						Order: 1, ID: "fix-image", Type: dorguv1.StepTypeWorkloadApply,
						Risk:        "medium",
						Description: "Correct the image tag in the Git manifests for the storefront ArgoCD application, then commit and let ArgoCD sync.",
						Command:     "kubectl describe deploy/frontend-web -n apps",
					},
					{
						Order: 2, ID: "read-events", Type: dorguv1.StepTypeManual,
						Description: "Read the pull error from the pod events.",
						Command:     "kubectl events -n apps --for pod/frontend-web-1",
					},
				},
				Approval: &dorguv1.ApprovalSpec{Required: true},
			},
			Status: dorguv1.RemediationActionStatus{Phase: "Pending"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "completed-checkout-cpu"},
			Spec: dorguv1.RemediationActionSpec{
				IncidentRef: dorguv1.IncidentReference{Name: "checkout-throttled", Namespace: "apps"},
				PersonaRef: dorguv1.PersonaReference{
					Kind: "ApplicationPersona", Name: "checkout", Namespace: "apps",
				},
				TrustLevel:  2,
				Confidence:  "0.91",
				PlanSource:  dorguv1.PlanSourceRuleBased,
				PlanSummary: "The podinfo container was CPU throttled at a 100m request.",
				Explanation: "Raised the CPU request to 200m.",
				WorkloadRef: helmWorkload,
				Action: dorguv1.RemediationActionDetail{
					Type:  dorguv1.ActionTypePersonaUpdate,
					Patch: rawPatch(`{"spec":{"resources":{"requests":{"cpu":"200m"}}}}`),
				},
				Steps: []dorguv1.RemediationStep{{
					Order: 1, ID: "raise-cpu", Type: dorguv1.StepTypePersonaUpdate,
					Risk: "low", AutoExecutable: true,
					Description:   "Set spec.resources.requests.cpu to 200m on the ApplicationPersona.",
					Patch:         rawPatch(`{"spec":{"resources":{"requests":{"cpu":"200m"}}}}`),
					PrePatchState: rawPatch(`{"spec":{"resources":{"requests":{"cpu":"100m"}}}}`),
				}},
				Approval: &dorguv1.ApprovalSpec{Required: true},
			},
			Status: dorguv1.RemediationActionStatus{
				Phase:              "Completed",
				ApprovedBy:         "cli-user",
				ApprovedAt:         &applied,
				AppliedAt:          &applied,
				VerificationResult: "Healthy",
				StepStatuses: []dorguv1.StepStatus{{
					Order: 1, Phase: "Verified", AppliedAt: &applied, VerificationResult: "Healthy",
				}},
				Conditions: []metav1.Condition{{
					Type:               "WorkloadPatched",
					Status:             metav1.ConditionTrue,
					Reason:             "CLIAppliedResourceChange",
					LastTransitionTime: applied,
					Message: "dorgu CLI applied resources.requests.cpu=200m to Deployment " +
						"apps/checkout-podinfo (container podinfo)",
				}},
			},
		},
	}
}

// The two guardrail messages, declared once because they appear twice: on the
// safety entry, and appended verbatim to the step description by the operator.
// The renderer takes them back out of the description, and it can only be seen
// doing that if the two copies are identical.
const (
	clampedMemoryMessage = "Blast-radius guardrail: the plan asked to set " +
		"spec.resources.limits.memory to 4Gi, which is 32.0x the 128Mi Dorgu observed on the " +
		"running container. The cap is 2.0x, so 256Mi will be applied instead."

	rejectedCPUMessage = `Left out: container "podinfo" on checkout does not set cpu today, ` +
		"and Dorgu will not introduce a resource key a workload has never had."
)

// rawPatch wraps a JSON literal as the CRD's apiextensions JSON type.
func rawPatch(json string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(json)}
}
