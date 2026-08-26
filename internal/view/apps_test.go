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

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dorguv1 "github.com/dorgu-ai/dorgu-operator/api/v1"
)

// ---------------------------------------------------------------------------
// builders
// ---------------------------------------------------------------------------

func persona(namespace, name, appName string, mutate ...func(*dorguv1.ApplicationPersona)) *dorguv1.ApplicationPersona {
	p := &dorguv1.ApplicationPersona{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: dorguv1.ApplicationPersonaSpec{
			Name: appName,
			Type: "api",
			Tier: "critical",
		},
	}
	for _, m := range mutate {
		m(p)
	}
	return p
}

func deploy(namespace, name string, mutate ...func(*appsv1.Deployment)) *appsv1.Deployment {
	replicas := int32(2)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: name, Image: "example/" + name + ":1.2.3"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2, AvailableReplicas: 2, UpdatedReplicas: 2},
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

func pod(namespace, name string, labels map[string]string, mutate ...func(*corev1.Pod)) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	for _, m := range mutate {
		m(p)
	}
	return p
}

func findApp(t *testing.T, payload AppsPayload, id string) App {
	t.Helper()
	for _, app := range payload.Apps {
		if app.ID == id {
			return app
		}
	}
	ids := make([]string, 0, len(payload.Apps))
	for _, app := range payload.Apps {
		ids = append(ids, app.ID)
	}
	t.Fatalf("no app with id %q; got %v", id, ids)
	return App{}
}

// ---------------------------------------------------------------------------
// the point of the view: naming the blind spot
// ---------------------------------------------------------------------------

// F-02: `dorgu health` reported "everything fine" on a cluster with three
// Deployments it had never been told about. Anything that only lists what Dorgu
// can already see reproduces that bug in a nicer font.
func TestUnmonitoredDeploymentsAppearWithAnImportCommand(t *testing.T) {
	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout"), deploy("apps", "legacy-billing")},
	})

	require.Len(t, payload.Apps, 2)
	assert.Equal(t, 1, payload.Summary.Monitored)
	assert.Equal(t, 1, payload.Summary.Unmonitored)
	assert.Equal(t, []string{"apps"}, payload.Summary.UnmonitoredNamespaces)

	unmonitored := findApp(t, payload, "deployment/apps/legacy-billing")
	assert.False(t, unmonitored.Monitored)
	assert.Equal(t, SourceDeployment, unmonitored.Source)
	assert.Equal(t, "dorgu persona import -n apps --name legacy-billing", unmonitored.ImportCommand)
	require.NotEmpty(t, unmonitored.Warnings)
	assert.Contains(t, unmonitored.Warnings[0], "not watching it")
}

func TestMonitoredDeploymentIsNotAlsoListedAsUnmonitored(t *testing.T) {
	// The brownfield shape: persona "frontend" resolving to Deployment
	// "frontend-podinfo" by selector, which is what Helm produces.
	deployment := deploy("apps", "frontend-podinfo", func(d *appsv1.Deployment) {
		d.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/name": "frontend"},
		}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "frontend", "frontend")},
		Deployments: []*appsv1.Deployment{deployment},
	})

	require.Len(t, payload.Apps, 1)
	app := payload.Apps[0]
	assert.True(t, app.Monitored)
	require.NotNil(t, app.Workload)
	assert.Equal(t, "frontend-podinfo", app.Workload.Name)
	assert.Equal(t, "spec.selector.matchLabels", app.Workload.MatchedBy,
		"the rung is shown so a reader can check the link rather than assume it")
}

// Every candidate of an ambiguous match is claimed, so an ambiguous persona
// does not also produce two "unmonitored" rows on top of its warning.
func TestAmbiguousMatchWarnsAndClaimsEveryCandidate(t *testing.T) {
	label := func(name string) func(*appsv1.Deployment) {
		return func(d *appsv1.Deployment) {
			d.Labels = map[string]string{"app.kubernetes.io/name": "frontend"}
			d.Name = name
		}
	}
	payload := BuildApps(AppsInput{
		Personas: []*dorguv1.ApplicationPersona{persona("apps", "frontend", "frontend")},
		Deployments: []*appsv1.Deployment{
			deploy("apps", "frontend-blue", label("frontend-blue")),
			deploy("apps", "frontend-green", label("frontend-green")),
		},
	})

	require.Len(t, payload.Apps, 1, "no unmonitored rows for the ambiguous candidates")
	app := payload.Apps[0]
	assert.Nil(t, app.Workload, "an ambiguous match must not pick a winner")
	require.Len(t, app.Warnings, 1)
	assert.Contains(t, app.Warnings[0], "2 Deployments match persona")
}

func TestPersonaWithNoDeploymentNamesTheRungsItTried(t *testing.T) {
	payload := BuildApps(AppsInput{
		Personas: []*dorguv1.ApplicationPersona{persona("apps", "ghost", "ghost")},
	})

	require.Len(t, payload.Apps, 1)
	app := payload.Apps[0]
	assert.Nil(t, app.Workload)
	require.Len(t, app.Warnings, 1)
	assert.Contains(t, app.Warnings[0], `matches persona name "ghost"`)
	assert.Contains(t, app.Warnings[0], "label app.kubernetes.io/name")
	assert.Equal(t, HealthSourceNone, app.Health.Source)
	assert.Equal(t, HealthUnknown, app.Health.Status)
}

func TestSystemNamespacesAreExcludedFromTheUnmonitoredScan(t *testing.T) {
	excluded := map[string]bool{"kube-system": true}

	all := BuildApps(AppsInput{
		Deployments:        []*appsv1.Deployment{deploy("kube-system", "coredns"), deploy("apps", "checkout")},
		ExcludedNamespaces: excluded,
	})
	require.Len(t, all.Apps, 1)
	assert.Equal(t, "apps", all.Apps[0].Namespace)

	// Asking for a namespace is asking for all of it, exclusions included.
	scoped := BuildApps(AppsInput{
		Deployments:        []*appsv1.Deployment{deploy("kube-system", "coredns"), deploy("apps", "checkout")},
		ExcludedNamespaces: excluded,
		Namespace:          "kube-system",
	})
	require.Len(t, scoped.Apps, 1)
	assert.Equal(t, "coredns", scoped.Apps[0].Name)
}

func TestNamespaceScopeFiltersPersonasAndDeployments(t *testing.T) {
	payload := BuildApps(AppsInput{
		Personas: []*dorguv1.ApplicationPersona{
			persona("apps", "checkout", "checkout"),
			persona("staging", "checkout", "checkout"),
		},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout"), deploy("staging", "checkout")},
		Namespace:   "apps",
	})

	require.Len(t, payload.Apps, 1)
	assert.Equal(t, "apps", payload.Apps[0].Namespace)
}

// ---------------------------------------------------------------------------
// health
// ---------------------------------------------------------------------------

func TestPersonaStatusHealthIsPreferredAndLabelled(t *testing.T) {
	checked := metav1.NewTime(time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC))
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{
			Status:    HealthDegraded,
			Message:   "1 of 2 replicas ready",
			LastCheck: &checked,
		}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthDegraded, health.Status)
	assert.Equal(t, HealthSourcePersona, health.Source)
	assert.Equal(t, "1 of 2 replicas ready", health.Message)
	require.NotNil(t, health.LastCheck)
}

// The case that matters most in this file.
//
// A persona's status is written on a reconcile interval, so an app that started
// crash-looping ten seconds ago still carries a Healthy verdict. Rendering that
// green next to a CrashLoopBackOff would reproduce the exact failure that made
// `dorgu health` report "everything fine" on a broken cluster.
func TestALiveCrashLoopOverridesAStaleHealthyVerdict(t *testing.T) {
	checked := metav1.NewTime(time.Date(2026, 8, 27, 3, 0, 0, 0, time.UTC))
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{
			Status:    HealthHealthy,
			Message:   "3 of 3 replicas ready",
			LastCheck: &checked,
		}
	})
	crashing := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:         "checkout",
			RestartCount: 7,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: "CrashLoopBackOff",
			}},
		}}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
		Pods:        []*corev1.Pod{crashing},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthUnhealthy, health.Status,
		"a green badge over a crash loop is the one lie this view must not tell")
	assert.Equal(t, HealthSourceObserved, health.Source)
	assert.Contains(t, health.Disagreement, "The operator last reported Healthy")
	assert.Contains(t, health.Disagreement, "can see worse")
	require.Len(t, health.PodIssues, 1)
	assert.Equal(t, 1, payload.Summary.Unhealthy,
		"the summary must count the honest status, not the stale one")
}

// The operator has history and a diagnosis; the dashboard has one moment. When
// the verdict is already the worse of the two it stands, and no disagreement is
// invented.
func TestAWorseOperatorVerdictStandsWithNoDisagreement(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{
			Status:  HealthUnhealthy,
			Message: "OOMKilled 4 times in the last hour",
		}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")}, // 2/2 ready
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthUnhealthy, health.Status)
	assert.Equal(t, HealthSourcePersona, health.Source)
	assert.Empty(t, health.Disagreement)
	assert.Equal(t, "OOMKilled 4 times in the last hour", health.Message)
}

func TestAnAgreeingVerdictReportsNoDisagreement(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{Status: HealthHealthy, Message: "all good"}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthHealthy, health.Status)
	assert.Equal(t, HealthSourcePersona, health.Source)
	assert.Empty(t, health.Disagreement)
}

// A verdict with no Deployment to check it against is taken as given: there is
// nothing to disagree with.
func TestAVerdictWithNoWorkloadIsTakenAsGiven(t *testing.T) {
	p := persona("apps", "ghost", "ghost", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{Status: HealthHealthy}
	})

	payload := BuildApps(AppsInput{Personas: []*dorguv1.ApplicationPersona{p}})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthHealthy, health.Status)
	assert.Equal(t, HealthSourcePersona, health.Source)
	assert.Empty(t, health.Disagreement)
}

// A scaled-to-zero app reads as Unknown. Unknown is the absence of a reading
// rather than evidence of trouble, so it must not override a Healthy verdict: an
// idle app is not a broken one, and a grey badge on it would not be honesty.
func TestScaledToZeroDoesNotOverrideAHealthyVerdict(t *testing.T) {
	p := persona("apps", "cron", "cron", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{Status: HealthHealthy, Message: "idle"}
	})
	idle := deploy("apps", "cron", func(d *appsv1.Deployment) {
		zero := int32(0)
		d.Spec.Replicas = &zero
		d.Status.ReadyReplicas = 0
		d.Status.AvailableReplicas = 0
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{idle},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthHealthy, health.Status)
	assert.Equal(t, HealthSourcePersona, health.Source)
	assert.Empty(t, health.Disagreement)
}

// A degraded observation is positive evidence and does override.
func TestADegradedObservationOverridesAHealthyVerdict(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{Status: HealthHealthy, Message: "2 of 2 ready"}
	})
	short := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Status.ReadyReplicas = 1
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{short},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthDegraded, health.Status)
	assert.Equal(t, HealthSourceObserved, health.Source)
	assert.Contains(t, health.Disagreement, "1/2 replicas ready")
}

// An Unhealthy verdict is not softened by a Degraded observation. The override
// only ever moves the reading in the less flattering direction.
func TestTheOverrideOnlyEverMovesTowardWorse(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{Status: HealthUnhealthy, Message: "OOMKilled"}
	})
	short := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Status.ReadyReplicas = 1
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{short},
	})

	assert.Equal(t, HealthUnhealthy, payload.Apps[0].Health.Status)
	assert.Equal(t, HealthSourcePersona, payload.Apps[0].Health.Source)
}

// With healthCheck disabled, or before the first reconcile, a persona carries no
// health. Reporting only Unknown while the Deployment plainly has zero ready
// replicas would be the blind-but-cheerful failure the CLI was fixed for.
func TestFallsBackToObservedHealthWhenThePersonaReportsNone(t *testing.T) {
	broken := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Status.ReadyReplicas = 0
		d.Status.AvailableReplicas = 0
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{broken},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthUnhealthy, health.Status)
	assert.Equal(t, HealthSourceObserved, health.Source)
	assert.Equal(t, "0/2 replicas ready.", health.Message)
}

func TestObservedHealthCases(t *testing.T) {
	tests := []struct {
		name        string
		desired     int32
		ready       int32
		wantStatus  string
		wantMessage string
	}{
		{"all ready", 2, 2, HealthHealthy, "2/2 replicas ready."},
		{"some ready", 3, 1, HealthDegraded, "1/3 replicas ready."},
		{"none ready", 2, 0, HealthUnhealthy, "0/2 replicas ready."},
		{"scaled to zero is a decision, not a fault", 0, 0, HealthUnknown, "Scaled to 0 replicas."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := deploy("apps", "checkout", func(d *appsv1.Deployment) {
				desired := tt.desired
				d.Spec.Replicas = &desired
				d.Status.ReadyReplicas = tt.ready
			})
			got := observedHealth(d, nil)
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantMessage, got.Message)
		})
	}
}

func TestNilSpecReplicasMeansOne(t *testing.T) {
	d := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Spec.Replicas = nil
		d.Status.ReadyReplicas = 1
	})
	assert.Equal(t, int32(1), desiredReplicas(d))
	assert.Equal(t, HealthHealthy, observedHealth(d, nil).Status)
}

// ---------------------------------------------------------------------------
// pod issues
// ---------------------------------------------------------------------------

func TestPodIssuesAreReadLiveFromContainerStates(t *testing.T) {
	crashing := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:         "checkout",
			RestartCount: 7,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off 5m0s restarting failed container",
			}},
		}}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
		Pods:        []*corev1.Pod{crashing},
	})

	health := payload.Apps[0].Health
	assert.Equal(t, HealthUnhealthy, health.Status)
	require.Len(t, health.PodIssues, 1)
	assert.Equal(t, "CrashLoopBackOff", health.PodIssues[0].Reason)
	assert.Equal(t, int32(7), health.PodIssues[0].RestartCount)
	assert.Contains(t, health.Message, "CrashLoopBackOff")
}

func TestStartupReasonsAreNotFailures(t *testing.T) {
	for _, reason := range []string{"ContainerCreating", "PodInitializing"} {
		t.Run(reason, func(t *testing.T) {
			p := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name:  "checkout",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
				}}
			})
			assert.Empty(t, podIssues(p))
		})
	}
}

// A container that was OOMKilled and came back is running now and would
// otherwise look fine. It is the single most important thing Dorgu detects.
func TestRestartedAfterOOMKillIsReportedWhileTheEvidenceLasts(t *testing.T) {
	p := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:         "checkout",
			RestartCount: 3,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				Reason:   "OOMKilled",
				ExitCode: 137,
			}},
		}}
	})

	issues := podIssues(p)
	require.Len(t, issues, 1)
	assert.Equal(t, "OOMKilled", issues[0].Reason)
	assert.Equal(t, int32(3), issues[0].RestartCount)
}

func TestTerminatedWithZeroExitIsNotAnIssue(t *testing.T) {
	p := pod("apps", "job-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "checkout",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
		}}
	})
	assert.Empty(t, podIssues(p))
}

func TestTerminatedWithNoReasonReportsTheExitCode(t *testing.T) {
	p := pod("apps", "checkout-abc", nil, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "checkout",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2}},
		}}
	})
	issues := podIssues(p)
	require.Len(t, issues, 1)
	assert.Equal(t, "Exited with code 2", issues[0].Reason)
}

func TestPodsAreMatchedBySelectorNotByNamespace(t *testing.T) {
	other := pod("apps", "unrelated-abc", map[string]string{"app": "billing"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "billing",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
		}}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
		Pods:        []*corev1.Pod{other},
	})

	assert.Empty(t, payload.Apps[0].Health.PodIssues,
		"another app's broken pod must not land on this row")
}

// An empty selector matches every pod in the namespace, which would attribute
// the whole namespace to one Deployment. Reporting nothing beats reporting wrong.
func TestEmptySelectorAttributesNoPods(t *testing.T) {
	d := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Spec.Selector = &metav1.LabelSelector{}
	})
	p := pod("apps", "anything", map[string]string{"app": "other"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "x",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}
	})
	assert.Empty(t, podIssuesFor(d, []*corev1.Pod{p}))
}

func TestMatchExpressionSelectorsAreHonoured(t *testing.T) {
	d := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Spec.Selector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "app",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"checkout"},
			}},
		}
	})
	p := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "checkout",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
		}}
	})

	issues := podIssuesFor(d, []*corev1.Pod{p})
	require.Len(t, issues, 1)
	assert.Equal(t, "ImagePullBackOff", issues[0].Reason)
}

// Live observation wins where both describe the same container, because a
// persona's recorded PodFailures can outlive the pod they describe.
func TestPersonaPodFailuresAreMergedWithoutDuplicating(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.Health = &dorguv1.HealthStatus{
			Status: HealthUnhealthy,
			PodFailures: []dorguv1.PodFailure{
				{PodName: "checkout-abc", Container: "checkout", Reason: "OOMKilled"},
				{PodName: "checkout-gone", Container: "checkout", Reason: "OOMKilled"},
			},
		}
	})
	live := pod("apps", "checkout-abc", map[string]string{"app": "checkout"}, func(p *corev1.Pod) {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "checkout",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
		Pods:        []*corev1.Pod{live},
	})

	issues := payload.Apps[0].Health.PodIssues
	require.Len(t, issues, 2)
	assert.Equal(t, "CrashLoopBackOff", issues[0].Reason, "the live reading wins")
	assert.Equal(t, "checkout-gone", issues[1].PodName)
}

// ---------------------------------------------------------------------------
// workload facts
// ---------------------------------------------------------------------------

func TestWorkloadCarriesOwnershipImageAndObservedResources(t *testing.T) {
	d := deploy("apps", "checkout", func(d *appsv1.Deployment) {
		d.Annotations = map[string]string{
			"meta.helm.sh/release-name":      "checkout",
			"meta.helm.sh/release-namespace": "apps",
		}
		d.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
		}
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{d},
	})

	w := payload.Apps[0].Workload
	require.NotNil(t, w)
	assert.Equal(t, dorguv1.ManagedByHelm, w.ManagedBy)
	assert.Equal(t, `Helm release "checkout" in namespace apps`, w.ManagedByDetail)
	assert.Equal(t, "example/checkout:1.2.3", w.Image)
	assert.Equal(t, "checkout", w.Container)
	assert.Equal(t, WorkloadReplicas{Desired: 2, Ready: 2, Available: 2, Updated: 2}, w.Replicas)

	require.NotNil(t, w.ObservedResources)
	require.NotNil(t, w.ObservedResources.Limits)
	assert.Equal(t, "128Mi", w.ObservedResources.Limits.Memory)
	assert.Empty(t, w.ObservedResources.Limits.CPU,
		"an absent key must stay empty: it is what forbids introducing one")
	require.NotNil(t, w.ObservedResources.Requests)
	assert.Equal(t, "100m", w.ObservedResources.Requests.CPU)
}

func TestNoResourceBlockMeansNoObservedResources(t *testing.T) {
	payload := BuildApps(AppsInput{
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
	})
	require.NotNil(t, payload.Apps[0].Workload)
	assert.Nil(t, payload.Apps[0].Workload.ObservedResources)
}

func TestPersonaOwnershipIsCarriedThrough(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Spec.Ownership = &dorguv1.OwnershipSpec{
			Team:   "payments",
			Owner:  "alex",
			OnCall: "payments-oncall",
		}
	})
	payload := BuildApps(AppsInput{Personas: []*dorguv1.ApplicationPersona{p}})

	require.NotNil(t, payload.Apps[0].Ownership)
	assert.Equal(t, "payments", payload.Apps[0].Ownership.Team)
	assert.Equal(t, "payments-oncall", payload.Apps[0].Ownership.OnCall)
}

func TestPersonaWithNoSpecNameFallsBackToMetadataName(t *testing.T) {
	p := persona("apps", "checkout", "")
	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
	})
	assert.Equal(t, "checkout", payload.Apps[0].AppName)
	require.NotNil(t, payload.Apps[0].Workload, "the fallback must still resolve a workload")
}

// ---------------------------------------------------------------------------
// incidents on an app row
// ---------------------------------------------------------------------------

func TestAppRowCountsOpenIncidentsAndKeepsThePersonaFigureSeparate(t *testing.T) {
	p := persona("apps", "checkout", "checkout", func(p *dorguv1.ApplicationPersona) {
		p.Status.ActiveIncidents = 9 // deliberately disagrees with the cache
	})

	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{p},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
		Incidents: []*dorguv1.IncidentMemory{
			incident("apps", "i1", "checkout", SeverityCritical, PhaseDetected),
			incident("apps", "i2", "checkout", SeverityWarning, PhaseDetected),
			incident("apps", "i3", "checkout", SeverityCritical, PhaseResolved),
		},
	})

	counts := payload.Apps[0].Incidents
	assert.Equal(t, 2, counts.Open, "resolved incidents are not open")
	assert.Equal(t, 1, counts.Critical)
	assert.Equal(t, int32(9), counts.PersonaReported,
		"a mismatch is a fact about the operator, not a rounding error to hide")
	assert.NotNil(t, counts.LastIncidentTime)
}

// ---------------------------------------------------------------------------
// ordering and summary
// ---------------------------------------------------------------------------

func TestWorstNewsSortsToTheTop(t *testing.T) {
	unhealthy := deploy("apps", "b-broken", func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 0 })
	degraded := deploy("apps", "a-degraded", func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 1 })
	healthy := deploy("apps", "a-fine")

	payload := BuildApps(AppsInput{
		Deployments: []*appsv1.Deployment{healthy, degraded, unhealthy},
	})

	got := []string{payload.Apps[0].Name, payload.Apps[1].Name, payload.Apps[2].Name}
	assert.Equal(t, []string{"b-broken", "a-degraded", "a-fine"}, got)
}

func TestSummaryMatchesTheRows(t *testing.T) {
	payload := BuildApps(AppsInput{
		Personas: []*dorguv1.ApplicationPersona{persona("apps", "checkout", "checkout")},
		Deployments: []*appsv1.Deployment{
			deploy("apps", "checkout"),
			deploy("apps", "broken", func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 0 }),
			deploy("staging", "half", func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 1 }),
		},
	})

	assert.Equal(t, 3, payload.Summary.Total)
	assert.Equal(t, len(payload.Apps), payload.Summary.Total)
	assert.Equal(t, 1, payload.Summary.Monitored)
	assert.Equal(t, 2, payload.Summary.Unmonitored)
	assert.Equal(t, 1, payload.Summary.Healthy)
	assert.Equal(t, 1, payload.Summary.Degraded)
	assert.Equal(t, 1, payload.Summary.Unhealthy)
	assert.Equal(t, []string{"apps", "staging"}, payload.Summary.UnmonitoredNamespaces)
}

func TestReadinessIsCarriedThroughUntouched(t *testing.T) {
	readiness := Readiness{Synced: true, CRDsInstalled: false, MissingCRDs: []string{"applicationpersonas"}}
	payload := BuildApps(AppsInput{Readiness: readiness})

	assert.Equal(t, readiness, payload.Readiness)
	assert.Empty(t, payload.Apps)
	assert.NotNil(t, payload.Apps, "an empty list must encode as [] rather than null")
}

func TestAppIDsAreStableAndUnique(t *testing.T) {
	// A persona and a Deployment can share a namespace and a name without being
	// the same row, so the source has to be part of the identity.
	payload := BuildApps(AppsInput{
		Personas:    []*dorguv1.ApplicationPersona{persona("apps", "checkout", "nothing-matches-this")},
		Deployments: []*appsv1.Deployment{deploy("apps", "checkout")},
	})

	require.Len(t, payload.Apps, 2)
	seen := map[string]bool{}
	for _, app := range payload.Apps {
		assert.False(t, seen[app.ID], "duplicate row id %q", app.ID)
		seen[app.ID] = true
	}
	assert.True(t, seen["persona/apps/checkout"])
	assert.True(t, seen["deployment/apps/checkout"])
}
