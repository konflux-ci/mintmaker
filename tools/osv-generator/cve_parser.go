package osv_generator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Download CSAF VEX file from given URL and store into a VEX struct
func GetVEXFromUrl(url string) (VEX, error) {
	resp, err := http.Get(url)
	if err != nil {
		return VEX{}, fmt.Errorf("could not fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return VEX{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return VEX{}, fmt.Errorf("could not read response body: %v", err)
	}

	var vexData VEX

	if err := json.Unmarshal([]byte(body), &vexData); err != nil {
		return VEX{}, fmt.Errorf("could not unmarshal JSON: %v", err)
	}

	fmt.Printf("Found %d vulnerabilities at %s\n", len(vexData.Vulnerabilities), url)
	return vexData, nil
}

// Convert VEX RPM data to OSV format
func ConvertToOSV(vexData VEX) []OSV {
	// Get list of affected packages
	affectedList := getAffectedList(vexData)

	var vulnerabilities []OSV
	for _, vulnerability := range vexData.Vulnerabilities {
		// Create OSV vulnerability object for each CVE
		osvVulnerability := OSV{
			SchemaVersion: "1.6.0",
			Id:            vulnerability.Cve,
			DatabaseSpecific: &DatabaseSpecific{
				Severity: vexData.Document.AggregateSeverity.Text,
				CWEids:   []string{vulnerability.Cwe.Id},
			},
			Modified:   time.Now().Format("2006-01-02T15:04:05Z"),
			Published:  getPublishedDate(vulnerability),
			Summary:    getSummary(vulnerability),
			Details:    getDetails(vulnerability),
			References: getReferencesList(vulnerability),
			Affected:   affectedList,
		}

		vulnerabilities = append(vulnerabilities, osvVulnerability)
	}
	return vulnerabilities
}

// Save all CVEs to an OSV file
func StoreToFile(filename string, convertedVulnerabilities []OSV) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("error accessing file: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)

	for _, v := range convertedVulnerabilities {
		if err := encoder.Encode(v); err != nil {
			return fmt.Errorf("could not encode OSV data: %v", err)
		}
	}

	return nil
}

// Get list of affected RPM packages from VEX data
func getAffectedList(vex VEX) []*Affected {
	var affectedList []*Affected

	// Traverse dependencies tree
	for _, branch := range vex.ProductTree.Branches {
		for _, subBranch := range branch.Branches {
			if subBranch.Category == "architecture" {
				for _, subSubBranch := range subBranch.Branches {
					// Collect only RPM dependencies
					if !strings.HasPrefix(subSubBranch.Product.ProductIdentificationHelper.Purl, "pkg:rpm") {
						continue
					}

					// Parse name and version from pURL
					re := regexp.MustCompile(`pkg:rpm(?:mod)?/([^@]+)@([^?]+)`)
					matches := re.FindStringSubmatch(subSubBranch.Product.ProductIdentificationHelper.Purl)
					purl, packageName, version := matches[0], matches[1], matches[2]

					affectedPackage := Affected{
						Package: &Package{
							Ecosystem: "Red Hat",
							Name:      packageName,
							Purl:      purl,
						},
						Ranges: []*Range{
							{
								Type: "ECOSYSTEM",
								Events: []*Event{
									{
										Introduced: "0.0.0",
									},
									{
										Fixed: version,
									},
								},
							},
						},
					}

					// There will be duplicated dependencied from different architectures, store data once
					if !contains(affectedList, affectedPackage) {
						affectedList = append(affectedList, &affectedPackage)
					}
				}
			}
		}
	}
	return affectedList
}

func getReferencesList(vulnerability *Vulnerability) []*Reference {
	var references []*Reference

	for _, reference := range vulnerability.References {
		if reference.Category == "self" {
			references = append(references, &Reference{
				Type: "REPORT",
				Url:  reference.Url,
			})
		} else {
			references = append(references, &Reference{
				Type: "WEB",
				Url:  reference.Url,
			})
		}
	}
	return references
}

func getDetails(vulnerability *Vulnerability) string {
	for _, note := range vulnerability.Notes {
		if note.Category == "description" {
			return note.Text
		}
	}
	panic("No CVE details found")
}

func getSummary(vulnerability *Vulnerability) string {
	for _, note := range vulnerability.Notes {
		if note.Category == "summary" {
			return note.Text
		}
	}
	panic("No CVE summary found")
}

func getPublishedDate(vulnerability *Vulnerability) string {
	t, err := time.Parse("2006-01-02T15:04:05+00:00", vulnerability.DiscoveryDate)
	if err != nil {
		fmt.Printf("Error parsing time for %s: %v\n", vulnerability.Cve, err)
		panic(err)
	}
	return t.Format("2006-01-02T15:04:05Z")
}

func contains(affectedList []*Affected, affectedPackage Affected) bool {
	for _, item := range affectedList {
		if item.Package.Name == affectedPackage.Package.Name {
			return true
		}
	}
	return false
}
