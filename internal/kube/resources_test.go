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

package kube

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
)

// realScopes mirrors how the operator's CRDs are actually declared.
// ClusterPersona is cluster-scoped; the other four are namespaced.
var realScopes = map[string]bool{
	ResourceApplicationPersonas: true,
	ResourceClusterPersonas:     false,
	ResourceIncidentMemories:    true,
	ResourceRemediationActions:  true,
	ResourceDorguEvents:         true,
}

// fakeDiscovery serves the given resources for the dorgu.io group version, with
// each one's real scope.
func fakeDiscovery(resources ...string) discovery.DiscoveryInterface {
	list := &metav1.APIResourceList{GroupVersion: GroupVersion.String()}
	for _, r := range resources {
		list.APIResources = append(list.APIResources,
			metav1.APIResource{Name: r, Namespaced: realScopes[r]})
	}
	return &discoveryfake.FakeDiscovery{
		Fake: &k8stesting.Fake{Resources: []*metav1.APIResourceList{list}},
	}
}

// erroringDiscovery fails every call, which is what an unreachable cluster does.
type erroringDiscovery struct {
	discovery.DiscoveryInterface
	err error
}

func (e *erroringDiscovery) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return nil, e.err
}

func TestDorguResourcesCoversTheFiveCRDs(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"applicationpersonas",
		"clusterpersonas",
		"incidentmemories",
		"remediationactions",
		"dorguevents",
	}, DorguResources)
}

// Taken from the operator's own api/v1 rather than restated, so a group rename
// cannot leave the dashboard watching a group that no longer exists.
func TestGroupVersionComesFromTheOperator(t *testing.T) {
	assert.Equal(t, "dorgu.io", GroupVersion.Group)
	assert.Equal(t, "v1", GroupVersion.Version)
	assert.Equal(t, "dorgu.io/v1", GroupVersion.String())
}

func TestGVR(t *testing.T) {
	gvr := GVR(ResourceIncidentMemories)
	assert.Equal(t, "dorgu.io", gvr.Group)
	assert.Equal(t, "v1", gvr.Version)
	assert.Equal(t, "incidentmemories", gvr.Resource)
}

func TestDiscoverFindsEveryInstalledResource(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery(DorguResources...))
	require.NoError(t, err)

	for _, r := range DorguResources {
		assert.True(t, present.Present(r), r)
	}
	assert.Empty(t, present.Missing())
}

// ClusterPersona is cluster-scoped. A namespaced list against it is answered
// with "the server could not find the requested resource", so the informer never
// syncs and startup hangs with nothing serving. The scope has to come from the
// server, which is what this pins.
func TestDiscoverReportsEachResourceScope(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery(DorguResources...))
	require.NoError(t, err)

	assert.False(t, present.Namespaced(ResourceClusterPersonas),
		"ClusterPersona is cluster-scoped; watching it with a namespace filter fails outright")
	for _, r := range []string{
		ResourceApplicationPersonas,
		ResourceIncidentMemories,
		ResourceRemediationActions,
		ResourceDorguEvents,
	} {
		assert.True(t, present.Namespaced(r), r)
	}
}

func TestAnAbsentResourceReportsNeitherPresentNorNamespaced(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery())
	require.NoError(t, err)

	assert.False(t, present.Present(ResourceClusterPersonas))
	assert.False(t, present.Namespaced(ResourceClusterPersonas))
	assert.False(t, present.Present("never-heard-of-it"))
}

func TestPresenceMapFlattensForTheStore(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery(ResourceApplicationPersonas))
	require.NoError(t, err)

	flat := present.PresenceMap()
	assert.True(t, flat[ResourceApplicationPersonas])
	assert.False(t, flat[ResourceClusterPersonas])
	assert.Len(t, flat, len(DorguResources))
	assert.Equal(t, present.Missing(), MissingResources(flat),
		"the two ways of asking what is missing must agree")
}

func TestDiscoverReportsAPartialInstall(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery(
		ResourceApplicationPersonas, ResourceIncidentMemories))
	require.NoError(t, err)

	assert.True(t, present.Present(ResourceApplicationPersonas))
	assert.False(t, present.Present(ResourceRemediationActions))
	assert.Equal(t,
		[]string{ResourceClusterPersonas, ResourceRemediationActions, ResourceDorguEvents},
		present.Missing(),
		"missing resources come back in DorguResources order so the message is stable")
}

// The group not being served at all means the operator's CRDs are not
// installed. That is an answer, not a failure: "you have no apps" and "the
// operator is not installed" are different statements and the UI says which.
func TestAGroupThatIsNotServedIsAnAnswerNotAnError(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery())
	require.NoError(t, err)

	for _, r := range DorguResources {
		assert.False(t, present.Present(r), r)
	}
	assert.Len(t, present.Missing(), len(DorguResources))
}

// Reporting "no CRDs" for an unreachable cluster would be a third wrong answer
// on top of the two above.
func TestAnUnreachableClusterIsAnError(t *testing.T) {
	_, err := DiscoverDorguResources(&erroringDiscovery{
		err: errors.New("dial tcp 10.0.0.1:443: i/o timeout"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovering dorgu.io/v1 resources")
	assert.Contains(t, err.Error(), "i/o timeout", "the underlying cause survives the wrap")
}

func TestDiscoverIgnoresResourcesOutsideTheWatchedSet(t *testing.T) {
	present, err := DiscoverDorguResources(fakeDiscovery(
		ResourceApplicationPersonas, "somethingelses"))
	require.NoError(t, err)

	assert.Len(t, present, len(DorguResources), "the map only ever holds watched resources")
	assert.NotContains(t, present, "somethingelses")
}

func TestMissingResourcesOnAnEmptyMapListsEverything(t *testing.T) {
	assert.Equal(t, DorguResources, MissingResources(map[string]bool{}))
}
