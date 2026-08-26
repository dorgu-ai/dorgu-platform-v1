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

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// deployment builds a Deployment with the given name, labels and selector.
func deployment(name string, labels map[string]string, selector map[string]string) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "apps", Labels: labels},
	}
	if selector != nil {
		d.Spec.Selector = &metav1.LabelSelector{MatchLabels: selector}
	}
	return d
}

func TestResolveWalksTheChainInOrder(t *testing.T) {
	tests := []struct {
		name        string
		deployments []*appsv1.Deployment
		persona     string
		wantName    string
		wantRung    string
	}{
		{
			name:        "app.kubernetes.io/name label",
			deployments: []*appsv1.Deployment{deployment("frontend-podinfo", map[string]string{LabelAppName: "frontend"}, nil)},
			persona:     "frontend",
			wantName:    "frontend-podinfo",
			wantRung:    RungLabelAppName,
		},
		{
			name:        "short app label",
			deployments: []*appsv1.Deployment{deployment("frontend-podinfo", map[string]string{LabelApp: "frontend"}, nil)},
			persona:     "frontend",
			wantName:    "frontend-podinfo",
			wantRung:    RungLabelApp,
		},
		{
			name:        "metadata.name",
			deployments: []*appsv1.Deployment{deployment("frontend", nil, nil)},
			persona:     "frontend",
			wantName:    "frontend",
			wantRung:    RungName,
		},
		{
			name: "selector matchLabels, the Helm and kustomize case",
			deployments: []*appsv1.Deployment{
				deployment("frontend-podinfo", nil, map[string]string{LabelAppName: "frontend"}),
			},
			persona:  "frontend",
			wantName: "frontend-podinfo",
			wantRung: RungSelector,
		},
		{
			name: "an earlier rung wins over a later one",
			deployments: []*appsv1.Deployment{
				deployment("by-name", nil, nil),
				deployment("by-label", map[string]string{LabelAppName: "by-name"}, nil),
			},
			persona:  "by-name",
			wantName: "by-label",
			wantRung: RungLabelAppName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rung, err := Resolve(tt.deployments, tt.persona)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantRung, rung)
		})
	}
}

func TestResolveNoMatchIsNotAnError(t *testing.T) {
	got, rung, err := Resolve([]*appsv1.Deployment{deployment("other", nil, nil)}, "frontend")
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Empty(t, rung)
}

func TestResolveEmptyPersonaNameMatchesNothing(t *testing.T) {
	got, _, err := Resolve([]*appsv1.Deployment{deployment("frontend", nil, nil)}, "")
	require.NoError(t, err)
	assert.Nil(t, got, "an empty persona name must not resolve by metadata.name")
}

// A tie is an error rather than a first-match-wins, because picking one
// arbitrarily is how a dashboard shows one Deployment's resources under another
// Deployment's app.
func TestResolveAmbiguousMatchIsAnError(t *testing.T) {
	deployments := []*appsv1.Deployment{
		deployment("frontend-blue", map[string]string{LabelAppName: "frontend"}, nil),
		deployment("frontend-green", map[string]string{LabelAppName: "frontend"}, nil),
	}

	got, rung, err := Resolve(deployments, "frontend")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, RungLabelAppName, rung)

	var ambiguous *AmbiguousError
	require.ErrorAs(t, err, &ambiguous)
	assert.Equal(t, []string{"frontend-blue", "frontend-green"}, ambiguous.Candidates,
		"candidates must be sorted so the message is stable")
	assert.Contains(t, ambiguous.Error(), "set app.kubernetes.io/name=frontend on exactly one")
}

func TestResolveSkipsNilDeployments(t *testing.T) {
	got, _, err := Resolve([]*appsv1.Deployment{nil, deployment("frontend", nil, nil)}, "frontend")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "frontend", got.Name)
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name    string
		deploy  *appsv1.Deployment
		persona string
		want    bool
	}{
		{"nil deployment", nil, "frontend", false},
		{"empty persona name", deployment("frontend", nil, nil), "", false},
		{"by name", deployment("frontend", nil, nil), "frontend", true},
		{"by label", deployment("x", map[string]string{LabelApp: "frontend"}, nil), "frontend", true},
		{"by selector", deployment("x", nil, map[string]string{LabelApp: "frontend"}), "frontend", true},
		{"no match", deployment("x", nil, nil), "frontend", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Matches(tt.deploy, tt.persona))
		})
	}
}

func TestChainDescriptionNamesEveryRung(t *testing.T) {
	got := ChainDescription()
	for _, rung := range []string{RungLabelAppName, RungLabelApp, RungName, RungSelector} {
		assert.Contains(t, got, rung, "an error message that omits a rung is a dead end")
	}
}

func TestPickContainer(t *testing.T) {
	withContainers := func(names ...string) *appsv1.Deployment {
		d := deployment("frontend-podinfo", nil, nil)
		for _, n := range names {
			d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers,
				corev1.Container{Name: n})
		}
		return d
	}

	t.Run("nil deployment", func(t *testing.T) {
		assert.Nil(t, PickContainer(nil, "frontend"))
	})

	t.Run("no containers", func(t *testing.T) {
		assert.Nil(t, PickContainer(withContainers(), "frontend"))
	})

	t.Run("exact name wins", func(t *testing.T) {
		got := PickContainer(withContainers("sidecar", "frontend"), "frontend")
		require.NotNil(t, got)
		assert.Equal(t, "frontend", got.Name)
	})

	// The brownfield case: persona "frontend" over a Deployment whose container
	// is called podinfo. Falling back to the first container is what lets Dorgu
	// report resources for an app whose names do not line up.
	t.Run("falls back to the first container", func(t *testing.T) {
		got := PickContainer(withContainers("podinfo", "sidecar"), "frontend")
		require.NotNil(t, got)
		assert.Equal(t, "podinfo", got.Name)
	})
}
