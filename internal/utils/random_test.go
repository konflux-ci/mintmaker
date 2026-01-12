// Copyright 2024 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import "testing"

func TestGetRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "should be able to generate one symbol rangom string",
			length: 1,
		},
		{
			name:   "should be able to generate rangom string",
			length: 5,
		},
		{
			name:   "should be able to generate long rangom string",
			length: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RandomString(tt.length)
			if len(got) != tt.length {
				t.Errorf("Got string %s has lenght %d but expected length is %d", got, len(got), tt.length)
			}
		})
	}
}
