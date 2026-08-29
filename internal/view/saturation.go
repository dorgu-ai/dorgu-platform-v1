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
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// Saturation arithmetic, ported from the CLI's health_saturation.go.
//
// It is a port rather than a shared package because the CLI does not depend on
// this module and this module does not depend on the CLI. The two have to agree
// about the same cluster or `dorgu health` and this screen will print different
// percentages for it, so the logic is copied deliberately and pinned by tests
// that mirror the CLI's. The right long-term fix is one package both import.
//
// The rule the whole file exists for: only pods a node has accepted hold an
// allocation. A pod with an empty spec.nodeName is in the scheduling queue and
// holds nothing anywhere, and because such a pod can request more than the
// cluster owns, counting it inflates the figure without limit. That is the whole
// of the 1689%.

// nodeCapacity is the allocatable pool: what the scheduler may hand out, summed
// over the nodes.
type nodeCapacity struct {
	cpu    resource.Quantity
	memory resource.Quantity
	nodes  int
}

// podClaims is what the scheduled pods have been granted from that pool.
type podClaims struct {
	cpu    resource.Quantity
	memory resource.Quantity
	// perNode is what each node is carrying, keyed by node name, for the
	// per-node columns.
	perNode map[string]*nodeClaim

	scheduled   int
	unscheduled int
}

// nodeClaim is one node's share of the claims, plus its pod count.
type nodeClaim struct {
	cpu    resource.Quantity
	memory resource.Quantity
	pods   int
}

// sumNodeCapacity sums allocatable CPU and memory over the node list.
//
// Allocatable rather than capacity: capacity includes what the kubelet and the
// OS reserve, which the scheduler will never hand to a pod, so requests over
// capacity understates how full the cluster is.
func sumNodeCapacity(nodes []*corev1.Node) nodeCapacity {
	out := nodeCapacity{nodes: len(nodes)}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			out.cpu.Add(*cpu)
		}
		if memory := node.Status.Allocatable.Memory(); memory != nil {
			out.memory.Add(*memory)
		}
	}
	return out
}

// sumPodClaims sums what the scheduled pods have claimed, and counts what was
// left out.
//
// Succeeded and Failed pods ran to completion and released their allocation.
// A pod with no spec.nodeName never had one, whether nothing can fit it, it is
// waiting on a node that has not joined, or it is scheduling-gated. The two
// exclusions are different facts and only the second is reported to the reader,
// because only the second is a problem.
func sumPodClaims(pods []*corev1.Pod) podClaims {
	out := podClaims{perNode: map[string]*nodeClaim{}}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if !podHoldsAllocation(pod) {
			if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
				out.unscheduled++
			}
			continue
		}

		out.scheduled++
		cpu, memory := podRequests(pod)
		out.cpu.Add(cpu)
		out.memory.Add(memory)

		claim := out.perNode[pod.Spec.NodeName]
		if claim == nil {
			claim = &nodeClaim{}
			out.perNode[pod.Spec.NodeName] = claim
		}
		claim.cpu.Add(cpu)
		claim.memory.Add(memory)
		claim.pods++
	}
	return out
}

// podHoldsAllocation reports whether a pod is holding resources on a node right
// now. It mirrors the operator's function of the same name.
//
// A Pending pod that has been bound to a node still counts: a reservation held
// while an image pulls is a real reservation.
func podHoldsAllocation(pod *corev1.Pod) bool {
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return false
	}
	return pod.Spec.NodeName != ""
}

// podRequests returns what one pod claims from its node: the sum of its app
// containers, floored by the largest single init container.
//
// That is how the scheduler accounts for a pod. Init containers run before the
// app containers rather than alongside them, so a pod needs whichever is larger,
// not both. This mirrors the operator's and the CLI's podRequests deliberately:
// two answers to the same question is worse than one answer in three places.
func podRequests(pod *corev1.Pod) (cpu, memory resource.Quantity) {
	for i := range pod.Spec.Containers {
		requests := pod.Spec.Containers[i].Resources.Requests
		cpu.Add(*requests.Cpu())
		memory.Add(*requests.Memory())
	}
	for i := range pod.Spec.InitContainers {
		requests := pod.Spec.InitContainers[i].Resources.Requests
		if initCPU := requests.Cpu(); initCPU.Cmp(cpu) > 0 {
			cpu = initCPU.DeepCopy()
		}
		if initMemory := requests.Memory(); initMemory.Cmp(memory) > 0 {
			memory = initMemory.DeepCopy()
		}
	}
	return cpu, memory
}

// buildSaturation assembles the reported figures.
//
// A resource with no allocatable pool gets no detail at all rather than a
// zero-denominator percentage: on a cluster whose nodes could not be listed
// there is nothing to be a share of, and 0% would be a claim about a cluster
// nobody measured.
func buildSaturation(capacity nodeCapacity, claims podClaims, usage store.NodeUsage) Saturation {
	out := Saturation{
		Nodes:           capacity.nodes,
		ScheduledPods:   claims.scheduled,
		UnscheduledPods: claims.unscheduled,
	}
	if !usage.Available() {
		out.UsedUnavailable = usage.Unavailable
	} else if !usage.ReadAt.IsZero() {
		readAt := metav1.NewTime(usage.ReadAt)
		out.UsedReadAt = &readAt
	}

	if capacity.cpu.MilliValue() > 0 {
		detail := &SaturationDetail{
			Allocatable:      formatCPU(capacity.cpu),
			Requested:        formatCPU(claims.cpu),
			RequestedPercent: percentOf(claims.cpu.MilliValue(), capacity.cpu.MilliValue()),
		}
		if used, ok := usage.CPU(); ok {
			detail.Used = formatCPU(used)
			percent := percentOf(used.MilliValue(), capacity.cpu.MilliValue())
			detail.UsedPercent = &percent
		}
		detail.Pressure = underPressure(claims.cpu.MilliValue(), capacity.cpu.MilliValue())
		out.CPU = detail
	}

	if capacity.memory.Value() > 0 {
		detail := &SaturationDetail{
			Allocatable:      formatMemory(capacity.memory),
			Requested:        formatMemory(claims.memory),
			RequestedPercent: percentOf(claims.memory.Value(), capacity.memory.Value()),
		}
		if used, ok := usage.Memory(); ok {
			detail.Used = formatMemory(used)
			percent := percentOf(used.Value(), capacity.memory.Value())
			detail.UsedPercent = &percent
		}
		detail.Pressure = underPressure(claims.memory.Value(), capacity.memory.Value())
		out.Memory = detail
	}

	return out
}

// underPressure reports whether requests are at or above the warn threshold.
//
// It is computed from the raw ratio rather than from the rendered percentage,
// and it rounds to a whole percent, because that is exactly what the CLI does
// before comparing against the same 90 threshold. Comparing the display value
// instead would put the two tools on different sides of the line for the same
// cluster: at a true 89.6% the CLI rounds to 90% and warns, and a one-decimal
// 89.6 does not. A dashboard and a CLI disagreeing about whether a cluster is
// full is worse than either answer.
func underPressure(part, whole int64) bool {
	if whole <= 0 {
		return false
	}
	return math.Round(float64(part)/float64(whole)*100) >= SaturationWarnPercent
}

// percentOf renders part/whole as a percentage, rounded to one decimal place.
//
// One decimal rather than a whole number: a meter is sized from this value, and
// a cluster moving from 25.4% to 25.6% should move the bar rather than sit still
// and then jump. A zero or negative denominator yields 0 and is only ever
// reached where the caller has already decided not to build a detail.
func percentOf(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*1000) / 10
}

// formatCPU renders CPU in millicores throughout, which is the unit both node
// allocatable and container requests are usually written in and the only one
// that stays readable for a 50m sidecar and a 64-core node at once.
func formatCPU(quantity resource.Quantity) string {
	return fmt.Sprintf("%dm", quantity.MilliValue())
}

// formatMemory renders bytes in the largest binary unit that keeps the number
// above 1, so a sum reads "1.4Gi" rather than "1503238553". It matches the CLI
// byte for byte, including the one decimal place on Gi and none below it.
func formatMemory(quantity resource.Quantity) string {
	const (
		ki = 1 << 10
		mi = 1 << 20
		gi = 1 << 30
	)
	bytes := quantity.Value()
	switch {
	case bytes >= gi:
		return fmt.Sprintf("%.1fGi", float64(bytes)/gi)
	case bytes >= mi:
		return fmt.Sprintf("%.0fMi", float64(bytes)/mi)
	case bytes >= ki:
		return fmt.Sprintf("%.0fKi", float64(bytes)/ki)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}
