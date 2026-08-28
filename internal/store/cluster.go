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

package store

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Node and metrics state, which is everything the Cluster view is built from
// that is not a dorgu.io object.
//
// Nodes are watched like every other kind. Node usage is not, and cannot be:
// the metrics.k8s.io API serves get and list and no watch verb at all, so it is
// polled. That is the one exception to this repo's watch-never-poll rule and it
// lives here rather than being hidden inside a view, so the exception is
// visible in the type that holds it.

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// PutNode stores a copy of the Node.
func (s *Store) PutNode(obj *corev1.Node) {
	if obj == nil {
		return
	}
	s.write(func() {
		s.nodes[keyOf(obj.Namespace, obj.Name)] = obj.DeepCopy()
	}, TopicCluster)
}

// DeleteNode removes a Node.
func (s *Store) DeleteNode(namespace, name string) {
	s.write(func() { delete(s.nodes, keyOf(namespace, name)) }, TopicCluster)
}

// Nodes returns copies of every stored Node.
func (s *Store) Nodes() []*corev1.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*corev1.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n.DeepCopy())
	}
	return out
}

// ---------------------------------------------------------------------------
// Node usage, from metrics-server
// ---------------------------------------------------------------------------

// NodeUsage is what the containers on the cluster's nodes are actually
// consuming, summed, plus when it was read.
//
// Unavailable is the field that makes this type honest. metrics-server is not
// installed by default on any managed Kubernetes offering, so the ordinary case
// is that there is no answer, and a zero CPU figure and an unmeasured one are
// not the same claim. Every consumer must check Available before reading CPU or
// Memory, which is why they are unexported behind a method rather than left as
// public fields anyone can read past.
type NodeUsage struct {
	cpu    resource.Quantity
	memory resource.Quantity

	// available reports that a measurement was taken. False means CPU and
	// Memory are zero values that mean nothing.
	available bool

	// Unavailable is the reason there is no measurement, ready to render. It is
	// non-empty exactly when available is false, so the UI can never print
	// "n/a ()" with an empty operand.
	Unavailable string

	// ReadAt is when the measurement was taken, so the UI can say how old it
	// is rather than implying it is live.
	ReadAt time.Time
}

// NewNodeUsage records a successful measurement.
func NewNodeUsage(cpu, memory resource.Quantity, readAt time.Time) NodeUsage {
	return NodeUsage{cpu: cpu, memory: memory, available: true, ReadAt: readAt}
}

// UnavailableNodeUsage records that there is no measurement, and why.
//
// An empty reason is replaced rather than passed through: "n/a ()" is the
// empty-operand defect the CLI's F-09 invariant exists to prevent, and the one
// place to stop it is where the absence is constructed.
func UnavailableNodeUsage(reason string) NodeUsage {
	if reason == "" {
		reason = "not reported"
	}
	return NodeUsage{Unavailable: reason}
}

// CPU returns the measured CPU usage and whether there is a measurement at all.
func (u NodeUsage) CPU() (resource.Quantity, bool) {
	return u.cpu, u.available
}

// Memory returns the measured memory usage and whether there is a measurement
// at all.
func (u NodeUsage) Memory() (resource.Quantity, bool) {
	return u.memory, u.available
}

// Available reports whether a measurement was taken.
func (u NodeUsage) Available() bool { return u.available }

// SetNodeUsage records the latest answer from metrics-server, or the latest
// reason there is none.
func (s *Store) SetNodeUsage(usage NodeUsage) {
	s.write(func() { s.nodeUsage = usage }, TopicCluster)
}

// NodeUsage returns the last recorded usage. The zero value reports itself as
// unavailable with a reason, so a view built before the first poll completes
// says "not measured yet" rather than zero.
func (s *Store) NodeUsage() NodeUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.nodeUsage.available && s.nodeUsage.Unavailable == "" {
		return UnavailableNodeUsage("metrics have not been read yet")
	}
	return s.nodeUsage
}
