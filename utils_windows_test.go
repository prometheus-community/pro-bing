// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows
// +build windows

package probing

import "testing"

func TestGetMessageLength(t *testing.T) {
	tests := []struct {
		description string
		pinger      *Pinger
		expected    int
	}{
		{
			description: "IPv4 total size < 2048",
			pinger: &Pinger{
				Size: 24, // default size
				ipv4: true,
			},
			expected: 2048,
		},
		{
			description: "IPv4 total size > 2048",
			pinger: &Pinger{
				Size: 1993, // 2048 - 2 * (ipv4.HeaderLen + 8) + 1
				ipv4: true,
			},
			expected: 2049,
		},
		{
			description: "IPv6 total size < 2048",
			pinger: &Pinger{
				Size: 24,
				ipv4: false,
			},
			expected: 2048,
		},
		{
			description: "IPv6 total size > 2048",
			pinger: &Pinger{
				Size: 1953, // 2048 - 2 * (ipv6.HeaderLen + 8) + 1
				ipv4: false,
			},
			expected: 2049,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			actual := tt.pinger.getMessageLength()
			if tt.expected != actual {
				t.Fatalf("unexpected message length, expected: %d, actual %d", tt.expected, actual)
			}
		})
	}
}
