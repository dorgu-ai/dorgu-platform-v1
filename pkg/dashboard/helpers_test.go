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

package dashboard

import (
	"net/http"

	"github.com/dorgu-ai/dorgu-platform-v1/internal/httpapi"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/sse"
	"github.com/dorgu-ai/dorgu-platform-v1/internal/store"
)

// testHandler builds the same handler Run serves, with an empty cache and no
// cluster behind it. Wiring the real thing is the point: it is how these tests
// catch a middleware that exists but was never mounted.
func testHandler() (http.Handler, error) {
	cache := store.New(nil)
	server, err := httpapi.New(httpapi.Config{
		Store:     cache,
		Snapshots: httpapi.NewSnapshots(cache, httpapi.SnapshotOptions{}),
		Broker:    sse.NewBroker(),
		Logger:    quietLogger(),
		Version:   "test",
	})
	if err != nil {
		return nil, err
	}
	return server.Handler(), nil
}
