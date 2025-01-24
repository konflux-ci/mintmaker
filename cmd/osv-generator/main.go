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
	dockerFilename := flag.String("docker-filename", "docker.nedb", "Filename for the Docker DB file")
	rpmFilename := flag.String("rpm-filename", "rpm.nedb", "Filename for the RPM DB file")
	destDir := flag.String("destination-dir", "/tmp/osv-offline", "Destination directory for the OSV DB files")
	days := flag.Int("days", 90, "Only advisories created in the last X days are included")

	flag.Parse()
	err := os.MkdirAll(*destDir, 0755)
	if err != nil {
		fmt.Println("failed to create destination path: ", err)
		os.Exit(1)
	}

	err = osv_downloader.DownloadOsvDb(*destDir)
	if err != nil {
		fmt.Println("Downloading the OSV database has failed: ", err)
		os.Exit(1)
	}

	osv_generator.GenerateOSV(filepath.Join(*destDir, *dockerFilename), true, *days)
	if err != nil {
		fmt.Println("Generating the container OSV database has failed: ", err)
		os.Exit(1)
	}
	osv_generator.GenerateOSV(filepath.Join(*destDir, *rpmFilename), false, *days)
	if err != nil {
		fmt.Println("Generating the RPM OSV database has failed: ", err)
		os.Exit(1)
	}

}
