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

package main

import (
	"flag"

	osv_generator "github.com/konflux-ci/mintmaker/tools/osv-generator"
)

// A demo which parses RPM CVE data created in the last 24 hours into OSV database format
func main() {
	filename := flag.String("filename", "redhat.nedb", "Output filename for OSV database")
	flag.Parse()

	osv_generator.GenerateOSV(*filename)
}
