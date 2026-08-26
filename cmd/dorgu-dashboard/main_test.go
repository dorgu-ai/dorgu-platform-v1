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

package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"  INFO ": slog.LevelInfo,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := parseLevel(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

// A typo in --log-level is a loud error rather than a silent fall back to info.
// Quietly ignoring it is how someone ends up debugging with logging off.
func TestParseLevelRejectsAnUnknownName(t *testing.T) {
	_, err := parseLevel("verbose")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log level")
	assert.Contains(t, err.Error(), "debug, info, warn or error")
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "localhost", []string{"localhost"}},
		{"several", "localhost,127.0.0.1", []string{"localhost", "127.0.0.1"}},
		{"spaces are trimmed", " a , b ", []string{"a", "b"}},
		{"a trailing comma is not a hostname", "a,b,", []string{"a", "b"}},
		{"empty entries are dropped", "a,,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitList(tt.input))
		})
	}
}
