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
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// assetPrefix is Vite's output directory for hashed bundles. Everything under it
// carries a content hash in its filename, so it can be cached forever; nothing
// else can.
const assetPrefix = "assets/"

// spaHandler serves the embedded single-page app.
//
// Unknown paths fall back to index.html because the client-side router owns
// them: /incidents is a route in the browser, not a file on disk. The fallback is
// deliberately not applied to the API prefix, which the mux has already claimed,
// so a typo in an API path returns a 404 rather than a page of HTML.
func spaHandler(assets fs.FS, logger *slog.Logger) http.Handler {
	if assets == nil {
		return notBuiltHandler(logger)
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return notBuiltHandler(logger)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}

		if info, err := fs.Stat(assets, name); err != nil || info.IsDir() {
			name = "index.html"
		}

		if strings.HasPrefix(name, assetPrefix) {
			// Content-hashed filenames, so a stale copy is impossible.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// index.html names the current bundle. Caching it is how a user ends
			// up running yesterday's frontend against today's API.
			w.Header().Set("Cache-Control", "no-store")
		}

		http.ServeFileFS(w, r, assets, name)
	})
}

// notBuiltMessage is what a browser gets when the Go binary was built without a
// frontend build.
//
// It is a real answer rather than a blank page or a 404. A Go-only build is a
// normal state while working on the backend, and the house rule is that a
// failure says what happened and what to do about it. The API still works, and
// the message says so, because that is the fastest way for someone to confirm
// the backend is fine and the frontend is simply absent.
const notBuiltMessage = `The dashboard UI is not in this binary.

The Go server is running and the API is live. The single-page app was not
embedded, which means this binary was built without a frontend build.

To build it:

    make web        # installs frontend deps and builds into internal/webui/dist
    make build      # rebuilds the binary with the UI embedded

The API is available regardless:

    GET /api/v1/apps
    GET /api/v1/incidents
    GET /api/v1/meta
    GET /api/v1/stream     (text/event-stream)
`

func notBuiltHandler(logger *slog.Logger) http.Handler {
	logged := false
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !logged {
			logged = true
			logger.Warn("no SPA embedded in this binary; serving an explanation instead",
				"hint", "run make web && make build")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		// 503 rather than 404: the resource is expected to exist and does not
		// yet, which is exactly what "service unavailable" means. A 404 would
		// suggest the URL was wrong.
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(notBuiltMessage))
	})
}
