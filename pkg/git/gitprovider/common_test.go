/*
Copyright 2024 Red Hat, Inc.

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

package gitprovider

import (
	"net/url"
	"testing"
)

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name    string
		gitUrl  string
		want    *url.URL
		wantErr bool
	}{
		{
			name:   "HTTPS URL",
			gitUrl: "https://github.com/user/repo.git",
			want: &url.URL{
				Scheme: "https",
				Host:   "github.com",
				Path:   "/user/repo",
			},
			wantErr: false,
		},
		{
			name:   "SSH URL",
			gitUrl: "git@github.com:user/repo.git",
			want: &url.URL{
				Scheme: "https",
				Host:   "github.com",
				Path:   "/user/repo",
			},
			wantErr: false,
		},
		{
			name:   "URL without .git suffix",
			gitUrl: "https://gitlab.company.com/user/repo",
			want: &url.URL{
				Scheme: "https",
				Host:   "gitlab.company.com",
				Path:   "/user/repo",
			},
			wantErr: false,
		},
		{
			name:    "Invalid URL",
			gitUrl:  "ht@tp://fake-url",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGitURL(tt.gitUrl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGitURL() error = %v, expected %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.want.String() {
				t.Errorf("ParseGitURL() result = %v, expected %v", got, tt.want)
			}
		})
	}
}
