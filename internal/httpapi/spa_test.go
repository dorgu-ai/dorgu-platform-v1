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

package httpapi

import (
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexIsServedAtTheRoot(t *testing.T) {
	h := newHarness(t, spaFS())

	rec := h.get(t, "/")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<!doctype html>")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"),
		"index.html names the current bundle; caching it serves yesterday's frontend")
}

// /incidents is a route in the browser, not a file on disk.
func TestClientSideRoutesFallBackToIndex(t *testing.T) {
	h := newHarness(t, spaFS())

	for _, path := range []string{"/apps", "/incidents", "/apps/apps/checkout", "/nope"} {
		t.Run(path, func(t *testing.T) {
			rec := h.get(t, path)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "<!doctype html>")
		})
	}
}

func TestHashedAssetsAreCachedForever(t *testing.T) {
	h := newHarness(t, spaFS())

	rec := h.get(t, "/assets/index-abc123.js")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Body.String(), "dorgu")
}

func TestPathTraversalCannotEscapeTheEmbeddedFS(t *testing.T) {
	h := newHarness(t, spaFS())

	for _, path := range []string{"/../etc/passwd", "/assets/../../etc/passwd", "//etc/passwd"} {
		t.Run(path, func(t *testing.T) {
			rec := h.get(t, path)
			// Either normalised into the SPA fallback or refused. Never a file
			// from outside the embedded filesystem.
			assert.NotContains(t, rec.Body.String(), "root:")
		})
	}
}

// A Go-only build is a normal state while working on the backend, and the house
// rule is that a failure says what happened and what to do about it.
func TestAMissingSPAServesAnExplanationNotABlankPage(t *testing.T) {
	h := newHarness(t, nil)

	rec := h.get(t, "/")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"503 says the resource is expected and absent; 404 would suggest a wrong URL")
	body := rec.Body.String()
	assert.Contains(t, body, "The dashboard UI is not in this binary")
	assert.Contains(t, body, "make web")
	assert.Contains(t, body, "/api/v1/apps",
		"the message points at the API so a reader can confirm the backend is fine")
}

// An assets FS that exists but has no index.html is the same situation as none
// at all, and must not serve a directory listing or a 404 loop.
func TestAnAssetsFSWithoutIndexIsTreatedAsUnbuilt(t *testing.T) {
	h := newHarness(t, fstest.MapFS{"assets/orphan.js": {Data: []byte("x")}})

	rec := h.get(t, "/")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "not in this binary")
}

// The API has to keep working with no UI embedded, because that is how someone
// confirms the backend is fine and the frontend is simply absent.
func TestTheAPIWorksWithNoSPAEmbedded(t *testing.T) {
	h := newHarness(t, nil)
	h.ready()

	assert.Equal(t, http.StatusOK, h.get(t, "/api/v1/apps").Code)
	assert.Equal(t, http.StatusOK, h.get(t, "/api/v1/meta").Code)
	assert.Equal(t, http.StatusOK, h.get(t, "/healthz").Code)
}

func TestADirectoryRequestFallsBackToIndex(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html>")},
		"assets/keep.txt": {Data: []byte("x")},
	}
	h := newHarness(t, assets)

	rec := h.get(t, "/assets")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<!doctype html>",
		"a directory must not produce a listing of the bundle")
}
