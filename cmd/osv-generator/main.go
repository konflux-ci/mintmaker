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
