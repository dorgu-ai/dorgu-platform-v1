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

package webui

import (
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireEmbeddedUI turns the tolerant assertion below into a strict one.
//
// Both states are normal locally: a fresh clone has no frontend build, and a
// release binary has one. In the release pipeline only the second is acceptable,
// and this is how that gets enforced rather than assumed.
const requireEmbeddedUI = "DORGU_REQUIRE_EMBEDDED_UI"

// Assets has to answer honestly in both states, because both are normal: a
// fresh clone with no frontend build, and a release binary with one. The test
// therefore asserts the invariant rather than one of the two answers.
func TestAssetsReportsWhetherTheUIIsPresent(t *testing.T) {
	assets, built := Assets()

	if !built {
		if os.Getenv(requireEmbeddedUI) != "" {
			t.Fatalf("%s is set but no SPA is embedded; run make web before make build",
				requireEmbeddedUI)
		}
		assert.Nil(t, assets, "an unbuilt binary must return no filesystem to serve from")
		t.Log("no frontend build in this binary; run make web to embed one")
		return
	}

	require.NotNil(t, assets)
	info, err := fs.Stat(assets, "index.html")
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Positive(t, info.Size(), "an empty index.html is not a build")
}

// The `all:` prefix on the embed directive is what lets a fresh clone compile,
// where dist holds only a dot-prefixed placeholder. If this ever fails, the
// directive lost its prefix and `go build` will break for anyone who has not run
// the frontend build.
func TestTheDistDirectoryIsAlwaysEmbeddable(t *testing.T) {
	entries, err := fs.ReadDir(embedded, "dist")
	require.NoError(t, err, "dist must exist in the embedded filesystem")
	assert.NotEmpty(t, entries, "go:embed requires at least one matching file")
}
