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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoContextKubeconfig is the shape a person who works on two clusters has.
const twoContextKubeconfig = `apiVersion: v1
kind: Config
current-context: dorgu-dev
clusters:
- name: dev
  cluster:
    server: https://dev.example.invalid
- name: staging
  cluster:
    server: https://staging.example.invalid
contexts:
- name: dorgu-dev
  context:
    cluster: dev
    user: dev-user
- name: dorgu-staging
  context:
    cluster: staging
    user: staging-user
users:
- name: dev-user
  user:
    token: dev-token
- name: staging-user
  user:
    token: staging-token
`

func writeKubeconfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

// isolate clears the ambient kubeconfig so a developer's own cluster cannot make
// these tests pass or fail.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
}

// Showing the context and server in the UI is not decoration: a dashboard that
// does not say which cluster it is describing is one you cannot trust when you
// have four of them.
func TestConnectReportsTheContextAndServerInUse(t *testing.T) {
	isolate(t)

	conn, err := Connect(ConnectOptions{Kubeconfig: writeKubeconfig(t, twoContextKubeconfig)})
	require.NoError(t, err)

	assert.Equal(t, "dorgu-dev", conn.Context)
	assert.Equal(t, "https://dev.example.invalid", conn.Server)
	assert.False(t, conn.InCluster)
}

func TestConnectHonoursAContextOverride(t *testing.T) {
	isolate(t)

	conn, err := Connect(ConnectOptions{
		Kubeconfig: writeKubeconfig(t, twoContextKubeconfig),
		Context:    "dorgu-staging",
	})
	require.NoError(t, err)

	assert.Equal(t, "dorgu-staging", conn.Context)
	assert.Equal(t, "https://staging.example.invalid", conn.Server)
}

func TestConnectReadsKUBECONFIGWhenNoPathIsGiven(t *testing.T) {
	isolate(t)
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContextKubeconfig))

	conn, err := Connect(ConnectOptions{})
	require.NoError(t, err)
	assert.Equal(t, "dorgu-dev", conn.Context)
}

// The informers list once and then watch, so client-go's default 5 QPS only ever
// makes the first paint slow for no reason.
func TestRateLimitsAreRaisedAboveTheClientGoDefault(t *testing.T) {
	isolate(t)

	conn, err := Connect(ConnectOptions{Kubeconfig: writeKubeconfig(t, twoContextKubeconfig)})
	require.NoError(t, err)

	assert.Equal(t, float32(DefaultQPS), conn.Config.QPS)
	assert.Equal(t, DefaultBurst, conn.Config.Burst)
	assert.Contains(t, conn.Config.UserAgent, "dorgu-dashboard",
		"an API server audit log should name what was reading it")
}

func TestExplicitRateLimitsAreHonoured(t *testing.T) {
	isolate(t)

	conn, err := Connect(ConnectOptions{
		Kubeconfig: writeKubeconfig(t, twoContextKubeconfig),
		QPS:        7,
		Burst:      9,
	})
	require.NoError(t, err)
	assert.Equal(t, float32(7), conn.Config.QPS)
	assert.Equal(t, 9, conn.Config.Burst)
}

// "unable to load configuration" with no path in it is the least useful error a
// Kubernetes tool can produce.
func TestAMissingKubeconfigFailsAndNamesTheFile(t *testing.T) {
	isolate(t)
	missing := filepath.Join(t.TempDir(), "nowhere", "config")

	_, err := Connect(ConnectOptions{Kubeconfig: missing})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable Kubernetes configuration")
	assert.Contains(t, err.Error(), missing)
}

func TestNoKubeconfigAnywhereMentionsTheInClusterFallbackItAlsoTried(t *testing.T) {
	isolate(t)

	_, err := Connect(ConnectOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "in-cluster fallback also failed",
		"both attempts are reported so the message is not misleading about what was tried")
}

func TestAnUnknownContextIsAnError(t *testing.T) {
	isolate(t)

	_, err := Connect(ConnectOptions{
		Kubeconfig: writeKubeconfig(t, twoContextKubeconfig),
		Context:    "no-such-context",
	})
	require.Error(t, err)
}
