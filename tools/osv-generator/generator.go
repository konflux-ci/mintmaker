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

package osv_generator

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const URL = "https://security.access.redhat.com/data/csaf/v2/advisories"

// Get a list of advisory filenames from releases.csv that were released
// within the specified number of days from the current time. The advisory
// filenames in releases.csv are sorted by released date, newest first.
func getAdvisoryListByReleases(days int) ([]string, error) {
	response, err := http.Get(fmt.Sprintf("%s/%s", URL, "releases.csv")) //nolint:gosec,noctx // fixed Red Hat security advisory URL
	if err != nil {
		fmt.Println("Error downloading file:", err)
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	// Load all advisories from the document
	csvReader := csv.NewReader(response.Body)
	records, err := csvReader.ReadAll()
	if err != nil {
		fmt.Println("Error parsing CSV:", err)
		return nil, err
	}

	var advisories []string
	dateThreshold := time.Now().AddDate(0, 0, -days)

	// Filter out those which are not older than the specified amount of `days`
	for _, record := range records {
		advisoryDate, err := time.Parse(time.RFC3339, record[1])
		if err != nil {
			fmt.Println("Error parsing date:", err)
			return nil, err
		}

		advisories = append(advisories, record[0])

		if advisoryDate.Before(dateThreshold) {
			// All recently modified advisories were found
			fmt.Printf("Found %d new advisories\n", len(advisories))
			break
		}
	}

	return advisories, nil
}

// Get list of new advisories sorted by name, returns maximum of `limit` advisories
func getAdvisoryListByModified(limit int) ([]string, error) {
	response, err := http.Get(fmt.Sprintf("%s/%s", URL, "changes.csv")) //nolint:gosec,noctx // fixed Red Hat security advisory URL
	if err != nil {
		fmt.Println("Error downloading file:", err)
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	// Load all advisories from the document
	csvReader := csv.NewReader(response.Body)
	records, err := csvReader.ReadAll()
	if err != nil {
		fmt.Println("Error parsing CSV:", err)
		return nil, err
	}

	// Sort records by name in descending order to get newest advisories first
	sort.Slice(records, func(i, j int) bool {
		return records[i][0] > records[j][0]
	})

	var advisories []string
	for _, record := range records {
		// Add only RHSA advisories for now
		if strings.Contains(record[0], "rhsa-") {
			advisories = append(advisories, record[0])
			if len(advisories) == limit {
				break
			}
		}
	}
	return advisories, nil
}

// Extract advisory data from the given URL, store as list of OSV objects
func extractAdvisory(advisory string, containerVulns bool) []OSV {
	vexVulnerability, err := GetVEXFromUrl(fmt.Sprintf("%s/%s", URL, advisory))
	if err != nil {
		panic(err)
	}
	convertedVulnerabilities := ConvertToOSV(vexVulnerability, containerVulns)
	return convertedVulnerabilities
}

// Generate OSV vulnerabilities from CSAF VEX data and store to a file
func GenerateOSV(filename string, containerVulns bool, days int) error {
	var osvList []OSV
	advisories, err := getAdvisoryListByReleases(days)
	if err != nil {
		return err
	}

	// Extract vulnerability data and convert to OSV format
	advisoryChan := make(chan []OSV)
	for _, advisory := range advisories {
		go func(advisory string) {
			advisoryChan <- extractAdvisory(advisory, containerVulns)
		}(advisory)
	}
	for range advisories {
		osvList = append(osvList, <-advisoryChan...)
	}

	if err := StoreToFile(filename, osvList); err != nil {
		return err
	}
	return nil
}
