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
	"fmt"
	"os"
	"path/filepath"

	osv_downloader "github.com/konflux-ci/mintmaker/tools/osv-downloader"
	osv_generator "github.com/konflux-ci/mintmaker/tools/osv-generator"
)

func main() {
	containerFilename := flag.String("container-filename", "docker.nedb", "Filename for the Container DB file")
	rpmFilename := flag.String("rpm-filename", "rpm.nedb", "Filename for the RPM DB file")
	destDir := flag.String("destination-dir", "/tmp/osv-offline", "Destination directory for the OSV DB files")
	days := flag.Int("days", 120, "Only advisories created in the last X days are included")

	flag.Parse()
	err := os.MkdirAll(*destDir, 0755) //nolint:gosec // need "other" permissions for the downloader to work
	if err != nil {
		fmt.Println("failed to create destination path: ", err)
		os.Exit(1)
	}

	err = osv_downloader.DownloadOsvDb(*destDir)
	if err != nil {
		fmt.Println("Downloading the OSV database has failed: ", err)
		os.Exit(1)
	}

	err = osv_generator.GenerateOSV(filepath.Join(*destDir, *containerFilename), true, *days)
	if err != nil {
		fmt.Println("Generating the container OSV database has failed: ", err)
		os.Exit(1)
	}
	err = osv_generator.GenerateOSV(filepath.Join(*destDir, *rpmFilename), false, *days)
	if err != nil {
		fmt.Println("Generating the RPM OSV database has failed: ", err)
		os.Exit(1)
	}

}
