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

// Package webui carries the built single-page app inside the binary.
//
// One binary, no asset directory to deploy alongside it and no separate origin
// to serve the frontend from. That is what makes `dorgu dashboard` a single
// command, and it is also why the frontend is Vite and not Next.js: a static
// bundle is exactly what go:embed can hold, and everything Next adds on top of
// one is discarded the moment it goes in here.
package webui

import (
	"embed"
	"io/fs"
)

// The dist directory is the Vite build output and is not checked in beyond a
// placeholder. The `all:` prefix is what lets this embed succeed on a fresh
// clone, where dist holds only the dotfile placeholder: without it, go:embed
// skips dot-prefixed names, matches nothing, and fails the build. So a
// checkout with no frontend build still compiles, and Assets reports that the
// UI is absent rather than serving a blank page.
//
//go:embed all:dist
var embedded embed.FS

// Assets returns the built SPA, and false when this binary has no frontend
// build in it.
func Assets() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
