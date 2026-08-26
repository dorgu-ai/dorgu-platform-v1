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
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZeroOptionsAreUsable(t *testing.T) {
	filled := Options{}.withDefaults()

	assert.Equal(t, DefaultHost, filled.Host)
	assert.Equal(t, DefaultVersion, filled.Version)
	require.NotNil(t, filled.Logger)
	require.NotNil(t, filled.OnReady)
	assert.NotPanics(t, func() { filled.OnReady("http://127.0.0.1:7171/") })
}

// Callers keep their Options and a run must not rewrite them.
func TestWithDefaultsDoesNotMutateTheReceiver(t *testing.T) {
	original := Options{}
	_ = original.withDefaults()

	assert.Empty(t, original.Host)
	assert.Empty(t, original.Version)
	assert.Nil(t, original.Logger)
	assert.Nil(t, original.OnReady)
}

func TestExplicitValuesSurviveDefaulting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	filled := Options{
		Host:    "127.0.0.2",
		Port:    9999,
		Version: "1.2.3",
		Logger:  logger,
	}.withDefaults()

	assert.Equal(t, "127.0.0.2", filled.Host)
	assert.Equal(t, 9999, filled.Port)
	assert.Equal(t, "1.2.3", filled.Version)
	assert.Same(t, logger, filled.Logger)
}

func TestLoopbackBindsValidateSilently(t *testing.T) {
	for _, host := range []string{"", "127.0.0.1", "127.0.0.2", "::1", "localhost"} {
		t.Run("host="+host, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

			require.NoError(t, Options{Host: host}.Validate(logger))
			assert.Empty(t, buf.String(), "a loopback bind is the expected case and needs no warning")
		})
	}
}

// A dashboard on 0.0.0.0 with no auth hands every reader on the network a live
// view of the cluster. The person who typed --host deserves to be told in words.
func TestNonLoopbackBindWarnsAndDemandsAnExplicitAllowlist(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	err := Options{Host: "0.0.0.0"}.Validate(logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Host header allowlist must be set explicitly")

	logged := buf.String()
	assert.Contains(t, logged, "unauthenticated")
	assert.Contains(t, logged, "0.0.0.0")
}

func TestNonLoopbackBindIsAllowedWithAnExplicitAllowlist(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	err := Options{Host: "0.0.0.0", AllowedHosts: []string{"dashboard.internal"}}.Validate(logger)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "unauthenticated",
		"allowing it does not make it quiet")
}

func TestValidateRejectsAnOutOfRangePort(t *testing.T) {
	assert.Error(t, Options{Port: -1}.Validate(nil))
	assert.Error(t, Options{Port: 70000}.Validate(nil))
	assert.NoError(t, Options{Port: 0}.Validate(nil), "0 asks the kernel for a free port")
	assert.NoError(t, Options{Port: 65535}.Validate(nil))
}

func TestValidateToleratesANilLogger(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = Options{Host: "0.0.0.0"}.Validate(nil)
	})
}

func TestIsLoopback(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1": true,
		"127.1.2.3": true,
		"::1":       true,
		"localhost": true,
		"":          false,
		"0.0.0.0":   false,
		"::":        false,
		"10.0.0.5":  false,
		"not-an-ip": false,
	}
	for host, want := range tests {
		t.Run("host="+host, func(t *testing.T) {
			assert.Equal(t, want, isLoopback(host))
		})
	}
}

func TestDisplayHostIsAlwaysClickable(t *testing.T) {
	assert.Equal(t, "localhost", displayHost(""))
	assert.Equal(t, "localhost", displayHost("0.0.0.0"))
	assert.Equal(t, "localhost", displayHost("::"))
	assert.Equal(t, "127.0.0.1", displayHost("127.0.0.1"))
}

// The default is loopback, and that is the whole security model. A change here
// is a change to the trust posture, not a tuning decision.
func TestTheDefaultBindIsLoopback(t *testing.T) {
	assert.True(t, isLoopback(DefaultHost))
	assert.Equal(t, "127.0.0.1", DefaultHost)
}
