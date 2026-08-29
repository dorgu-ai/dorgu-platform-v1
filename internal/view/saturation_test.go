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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// node builds a Node with the allocatable pool the tests measure against.
func node(name, cpu, memory string, mutate ...func(*corev1.Node)) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(baseTime),
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          "v1.33.4",
				ContainerRuntimeVersion: "containerd://2.1.4",
				OperatingSystem:         "linux",
				Architecture:            "arm64",
			},
		},
	}
	for _, m := range mutate {
		m(n)
	}
	return n
}

// claimingPod builds a Pod claiming cpu and memory. An empty nodeName is the whole point
// of several tests below: it is a pod in the scheduling queue, holding nothing.
func claimingPod(name, nodeName, cpu, memory string, mutate ...func(*corev1.Pod)) *corev1.Pod {
	requests := corev1.ResourceList{}
	if cpu != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: name},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{Name: "app", Resources: corev1.ResourceRequirements{Requests: requests}},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	for _, m := range mutate {
		m(p)
	}
	return p
}

func usage(cpu, memory string) store.NodeUsage {
	return store.NewNodeUsage(resource.MustParse(cpu), resource.MustParse(memory), baseTime)
}

// ---------------------------------------------------------------------------
// The regression this whole view was gated on
// ---------------------------------------------------------------------------

// The reported defect, reproduced with the reported numbers.
//
// `dorgu health` printed 1689% CPU on a cluster where 25% was requested and 1%
// was in use, because the operator summed the requests of every non-terminal pod
// including the ones no node had accepted. A pod in the scheduling queue holds no
// allocation anywhere, and because it can ask for more than the cluster owns the
// error has no upper bound. This is the test that has to keep failing if anyone
// puts those pods back.
func TestUnscheduledPodsAreExcludedFromSaturation(t *testing.T) {
	nodes := []*corev1.Node{
		node("node-a", "1930m", "3600Mi"),
		node("node-b", "1930m", "3600Mi"),
	}

	pods := []*corev1.Pod{
		claimingPod("checkout", "node-a", "500m", "512Mi"),
		claimingPod("reports", "node-b", "450m", "320Mi"),
		// Three pods nobody can place, each asking for far more than the whole
		// cluster owns. Counting even one of these is what produced the 1689%.
		claimingPod("batch-1", "", "21", "1Gi"),
		claimingPod("batch-2", "", "21", "1Gi"),
		claimingPod("batch-3", "", "21", "1Gi"),
	}

	got := buildSaturation(sumNodeCapacity(nodes), sumPodClaims(pods), usage("38m", "1010Mi"))

	require.NotNil(t, got.CPU)
	assert.Equal(t, "3860m", got.CPU.Allocatable)
	assert.Equal(t, "950m", got.CPU.Requested)
	assert.InDelta(t, 24.6, got.CPU.RequestedPercent, 0.1,
		"only the two scheduled pods hold an allocation")
	assert.Less(t, got.CPU.RequestedPercent, 100.0,
		"a saturation figure over 100% of allocatable is arithmetically impossible")

	assert.Equal(t, 2, got.ScheduledPods)
	assert.Equal(t, 3, got.UnscheduledPods,
		"the excluded pods are reported, because they are the real problem the old number buried")
	assert.Equal(t, 2, got.Nodes)
}

// Succeeded and Failed pods released their allocation, so they are excluded too.
// They are NOT reported as unscheduled: a completed job is not a pod that cannot
// be placed, and folding the two would put a warning on a healthy cluster.
func TestTerminalPodsAreExcludedAndNotReportedAsUnscheduled(t *testing.T) {
	pods := []*corev1.Pod{
		claimingPod("running", "node-a", "100m", "128Mi"),
		claimingPod("done", "node-a", "8", "8Gi", func(p *corev1.Pod) {
			p.Status.Phase = corev1.PodSucceeded
		}),
		claimingPod("crashed", "node-a", "8", "8Gi", func(p *corev1.Pod) {
			p.Status.Phase = corev1.PodFailed
		}),
	}

	claims := sumPodClaims(pods)
	assert.Equal(t, 1, claims.scheduled)
	assert.Equal(t, 0, claims.unscheduled,
		"a pod that ran to completion is not a pod that cannot be scheduled")
	assert.Equal(t, int64(100), claims.cpu.MilliValue())
}

// A Pending pod that HAS been bound to a node still counts. A reservation held
// while an image pulls is a real reservation, and dropping it would understate
// how full the cluster is during exactly the window a rollout happens in.
func TestPendingButScheduledPodStillHoldsItsAllocation(t *testing.T) {
	pods := []*corev1.Pod{
		claimingPod("pulling", "node-a", "250m", "256Mi", func(p *corev1.Pod) {
			p.Status.Phase = corev1.PodPending
		}),
	}

	claims := sumPodClaims(pods)
	assert.Equal(t, 1, claims.scheduled)
	assert.Equal(t, int64(250), claims.cpu.MilliValue())
}

// ---------------------------------------------------------------------------
// Requested and used are two different numbers
// ---------------------------------------------------------------------------

// The second half of the same defect. The CRD field was called usedCPU and held
// requests, so one line could not say which it meant. On the reported cluster
// those were 25% and 1%: the difference between nearly full and nearly idle.
func TestRequestedAndUsedAreReportedSeparately(t *testing.T) {
	nodes := []*corev1.Node{node("node-a", "3860m", "7Gi")}
	pods := []*corev1.Pod{claimingPod("checkout", "node-a", "950m", "832Mi")}

	got := buildSaturation(sumNodeCapacity(nodes), sumPodClaims(pods), usage("38m", "1010Mi"))

	require.NotNil(t, got.CPU)
	assert.Equal(t, "950m", got.CPU.Requested)
	assert.Equal(t, "38m", got.CPU.Used)
	require.NotNil(t, got.CPU.UsedPercent)
	assert.NotEqual(t, got.CPU.RequestedPercent, *got.CPU.UsedPercent,
		"requested and used answer different questions and must not share a value")
	assert.Empty(t, got.UsedUnavailable)
	require.NotNil(t, got.UsedReadAt)
}

// metrics-server is not installed by default anywhere, so this is the ordinary
// case. A nil UsedPercent is the encoding that makes rendering zero impossible
// rather than merely discouraged, and the reason travels with it.
func TestAbsentMetricsLeaveUsedNilWithAReason(t *testing.T) {
	nodes := []*corev1.Node{node("node-a", "3860m", "7Gi")}
	pods := []*corev1.Pod{claimingPod("checkout", "node-a", "950m", "832Mi")}

	got := buildSaturation(sumNodeCapacity(nodes), sumPodClaims(pods),
		store.UnavailableNodeUsage("metrics-server did not answer: the server could not find the requested resource"))

	require.NotNil(t, got.CPU)
	assert.Equal(t, "950m", got.CPU.Requested, "requested reports without metrics-server")
	assert.Nil(t, got.CPU.UsedPercent, "no measurement must not render as 0%")
	assert.Empty(t, got.CPU.Used)
	assert.Contains(t, got.UsedUnavailable, "metrics-server did not answer")
	assert.Nil(t, got.UsedReadAt)
}

// An unavailable reading always carries a reason, so the UI can never print
// "n/a ()" with an empty operand.
func TestUnavailableUsageAlwaysCarriesAReason(t *testing.T) {
	got := buildSaturation(
		sumNodeCapacity([]*corev1.Node{node("node-a", "1", "1Gi")}),
		sumPodClaims(nil),
		store.UnavailableNodeUsage(""),
	)
	assert.NotEmpty(t, got.UsedUnavailable)
}

// ---------------------------------------------------------------------------
// Pod accounting
// ---------------------------------------------------------------------------

// The scheduler needs whichever is larger, not both: init containers run before
// the app containers rather than alongside them. This mirrors the operator's and
// the CLI's accounting on purpose.
func TestInitContainersFloorRatherThanAddToPodRequests(t *testing.T) {
	p := claimingPod("migrate", "node-a", "100m", "128Mi", func(p *corev1.Pod) {
		p.Spec.InitContainers = []corev1.Container{{
			Name: "migrate",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			}},
		}}
	})

	cpu, memory := podRequests(p)
	assert.Equal(t, int64(500), cpu.MilliValue(),
		"the larger init container floors the pod's CPU claim")
	expectedMemory := resource.MustParse("128Mi")
	assert.Equal(t, expectedMemory.Value(), memory.Value(),
		"the app containers already ask for more memory than the init container")
}

// A container with no requests genuinely requests nothing, which is a real
// answer and not a missing one.
func TestPodWithNoRequestsClaimsNothing(t *testing.T) {
	claims := sumPodClaims([]*corev1.Pod{claimingPod("bare", "node-a", "", "")})
	assert.Equal(t, 1, claims.scheduled)
	assert.Equal(t, int64(0), claims.cpu.MilliValue())
}

// ---------------------------------------------------------------------------
// Nothing to be a share of
// ---------------------------------------------------------------------------

// With no nodes there is no allocatable pool, so there is no percentage to
// report. A detail with a zero denominator would render as 0% of nothing, which
// is a claim about a cluster nobody measured.
func TestNoNodesMeansNoSaturationDetailAtAll(t *testing.T) {
	got := buildSaturation(sumNodeCapacity(nil), sumPodClaims(nil), usage("0", "0"))
	assert.Nil(t, got.CPU)
	assert.Nil(t, got.Memory)
	assert.Equal(t, 0, got.Nodes)
}

// ---------------------------------------------------------------------------
// Pressure
// ---------------------------------------------------------------------------

// At or above 90% requested the view stops being neutral, matching the CLI's
// threshold so the two agree about the same cluster.
//
// The boundary cases are the point. The CLI rounds the ratio to a whole percent
// before comparing, so a true 89.6% is 90% to it and warns. A dashboard that
// compared its own one-decimal display value would not warn there, and the two
// tools would disagree about whether the same cluster is full.
func TestPressureIsFlaggedAtTheSameThresholdAsTheCLI(t *testing.T) {
	tests := []struct {
		name         string
		requestedCPU string
		wantPressure bool
	}{
		{"well under", "500m", false},
		{"under, and rounds down", "894m", false},
		{"under, but rounds up to the threshold", "896m", true},
		{"just under the boundary", "895m", true},
		{"exactly at the threshold", "900m", true},
		{"over", "980m", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := []*corev1.Node{node("node-a", "1000m", "1Gi")}
			pods := []*corev1.Pod{claimingPod("app", "node-a", tt.requestedCPU, "1Mi")}

			got := buildSaturation(sumNodeCapacity(nodes), sumPodClaims(pods),
				store.UnavailableNodeUsage("no metrics-server"))
			require.NotNil(t, got.CPU)
			assert.Equal(t, tt.wantPressure, got.CPU.Pressure)
		})
	}
}

// ---------------------------------------------------------------------------
// Formatting, which has to match the CLI byte for byte
// ---------------------------------------------------------------------------

func TestQuantityFormattingMatchesTheCLI(t *testing.T) {
	tests := []struct {
		quantity string
		wantCPU  string
		wantMem  string
	}{
		{"0", "0m", "0"},
		{"1500m", "1500m", ""},
		{"3860m", "3860m", ""},
	}
	for _, tt := range tests {
		if tt.wantCPU != "" {
			assert.Equal(t, tt.wantCPU, formatCPU(resource.MustParse(tt.quantity)))
		}
	}

	assert.Equal(t, "1.0Gi", formatMemory(resource.MustParse("1Gi")))
	assert.Equal(t, "7.1Gi", formatMemory(resource.MustParse("7275Mi")))
	assert.Equal(t, "832Mi", formatMemory(resource.MustParse("832Mi")))
	assert.Equal(t, "512Ki", formatMemory(resource.MustParse("512Ki")))
	assert.Equal(t, "512", formatMemory(resource.MustParse("512")))
}

// A percentage is a number in this payload, not a string, because the UI sizes a
// meter from it. One decimal place, so a slow drift moves the bar instead of
// sitting still and then jumping.
func TestPercentOfKeepsOneDecimalAndRefusesAZeroDenominator(t *testing.T) {
	assert.Equal(t, 24.6, percentOf(950, 3860))
	assert.Equal(t, 100.0, percentOf(10, 10))
	assert.Equal(t, 0.0, percentOf(10, 0), "a zero denominator yields no percentage, not infinity")
	assert.Equal(t, 0.0, percentOf(10, -1))
}

// The zero NodeUsage has to report itself as unavailable with a reason, so a view
// built before the first poll completes says "not measured yet" rather than zero.
func TestZeroNodeUsageReportsItselfUnavailable(t *testing.T) {
	s := store.New(nil)
	got := s.NodeUsage()
	assert.False(t, got.Available())
	assert.NotEmpty(t, got.Unavailable)

	s.SetNodeUsage(store.NewNodeUsage(resource.MustParse("100m"), resource.MustParse("1Gi"), time.Now()))
	got = s.NodeUsage()
	require.True(t, got.Available())
	cpu, ok := got.CPU()
	require.True(t, ok)
	assert.Equal(t, int64(100), cpu.MilliValue())
}
