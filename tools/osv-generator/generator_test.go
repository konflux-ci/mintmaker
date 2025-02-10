package osv_generator

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jarcoal/httpmock"
)

var sampleCSV = []byte(`2023/rhsa-2023_6919.json,2024-12-02T08:22:15+00:00
2023/rhba-2023_6330.json,2024-12-02T08:22:03+00:00
2022/rhba-2022_5476.json,2024-12-02T07:52:26+00:00
2022/rhsa-2022_5249.json,2024-12-02T07:52:17+00:00
2024/rhsa-2024_5439.json,2024-12-02T07:52:10+00:00`)

var sampleOSVResultRPMs = `{"_id":"abcd","schema_version":"1.6.0","id":"CVE","database_specific":{"severity":"Moderate","cwe_ids":["CWE"]},"published":"2024-08-20T10:54:54Z","summary":"summary","details":"description","affected":[{"package":{"ecosystem":"Red Hat","name":"redhat/openstack","purl":"pkg:rpm/redhat/openstack@2.0.0"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0.0.0"},{"fixed":"2.0.0"}]}]}],"references":[{"type":"WEB","url":"fake-url"}]}`
var sampleOSVResultContainers = `{"_id":"abcd","schema_version":"1.6.0","id":"CVE","database_specific":{"severity":"Moderate","cwe_ids":["CWE"]},"published":"2024-08-20T10:54:54Z","summary":"summary","details":"description","affected":[{"package":{"ecosystem":"Docker","name":"some-registry.com/org/repo","purl":"pkg:oci/test-image@sha256:abcd?arch=amd64\u0026repository_url=some-registry.com/org/repo\u0026tag=v1"},"ranges":[]}],"references":[{"type":"WEB","url":"fake-url"}]}`

func TestGetAdvisoryListByModified(t *testing.T) {
	// Create a mock
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)
	httpmock.RegisterResponder("GET", "https://security.access.redhat.com/data/csaf/v2/advisories/changes.csv",
		httpmock.NewBytesResponder(200, sampleCSV))

	advisories, err := getAdvisoryListByModified(10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{"2024/rhsa-2024_5439.json", "2023/rhsa-2023_6919.json", "2022/rhsa-2022_5249.json"}
	if !cmp.Equal(advisories, expected) {
		t.Fatalf("expected different result: %v", cmp.Diff(expected, advisories))
	}
}

// E2E test of the whole functionality of OSV module
func TestGenerateOSVRPMs(t *testing.T) {
	// Create a mock
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)
	httpmock.RegisterResponder("GET", "https://security.access.redhat.com/data/csaf/v2/advisories/changes.csv",
		httpmock.NewBytesResponder(200, []byte(`test-rhsa-advisory.json,2024-12-02T07:52:10+00:00`)))
	// Set up a fake advisory
	httpmock.RegisterResponder("GET", "https://security.access.redhat.com/data/csaf/v2/advisories/test-rhsa-advisory.json",
		httpmock.NewBytesResponder(200, []byte(`{
			"document": {
				"aggregate_severity": {
					"text": "Moderate"
				}
			},
			"vulnerabilities": [
				{
					"cve": "CVE",
					"discovery_date": "2024-08-20T10:54:54.042000+00:00",
					"cwe": {
						"id": "CWE"
					},
					"references": [
						{
							"category": "REPORT",
							"url": "fake-url"
						}
					],
					"notes": [
						{
							"category": "summary",
							"text": "summary"
						},
						{
							"category": "description",
							"text": "description"
						}
					]
				}
			],
			"product_tree": {
				"branches": [
					{
						"branches": [
							{
								"category": "architecture",
								"branches": [
									{
										"product": {
											"product_identification_helper": {
												"purl": "pkg:rpm/redhat/openstack@2.0.0?arch=src"
											}
										}
									}
								]
							}
						]
					}
				]
			}
		}`)))

	// Create a test file
	err := GenerateOSV("testfile", false, 100000)
	defer os.Remove("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	createdFile, err := os.ReadFile("testfile")
	if err != nil {
		t.Fatalf("expected no error reading the file, got %v", err)
	}

	// Remove the modified field since it is set to current time
	re := regexp.MustCompile(`"modified":"[^"]*",`)
	result := re.ReplaceAllString(string(createdFile), "")
	// Replace randomly generated internal ID to a specific one for easier comparison
	re = regexp.MustCompile(`"_id":"[^"]*"`)
	result = re.ReplaceAllString(result, `"_id":"abcd"`)
	// Remove newlines
	result = strings.ReplaceAll(result, "\n", "")

	// Compare the results
	if result != sampleOSVResultRPMs {
		t.Fatalf("expected different result: %v", cmp.Diff(string(sampleOSVResultRPMs), string(result)))
	}

}

// E2E test of the whole functionality of OSV module
func TestGenerateOSVContainers(t *testing.T) {
	// Create a mock
	httpmock.Activate()
	t.Cleanup(httpmock.DeactivateAndReset)
	httpmock.RegisterResponder("GET", "https://security.access.redhat.com/data/csaf/v2/advisories/changes.csv",
		httpmock.NewBytesResponder(200, []byte(`test-rhsa-advisory.json,2024-12-02T07:52:10+00:00`)))
	// Set up a fake advisory
	httpmock.RegisterResponder("GET", "https://security.access.redhat.com/data/csaf/v2/advisories/test-rhsa-advisory.json",
		httpmock.NewBytesResponder(200, []byte(`{
			"document": {
				"aggregate_severity": {
					"text": "Moderate"
				}
			},
			"vulnerabilities": [
				{
					"cve": "CVE",
					"discovery_date": "2024-08-20T10:54:54.042000+00:00",
					"cwe": {
						"id": "CWE"
					},
					"references": [
						{
							"category": "REPORT",
							"url": "fake-url"
						}
					],
					"notes": [
						{
							"category": "summary",
							"text": "summary"
						},
						{
							"category": "description",
							"text": "description"
						}
					]
				}
			],
			"product_tree": {
				"branches": [
					{
						"branches": [
							{
								"category": "architecture",
								"branches": [
									{
										"product": {
											"product_identification_helper": {
												"purl": "pkg:oci/test-image@sha256:abcd?arch=amd64&repository_url=some-registry.com/org/repo&tag=v1"
											}
										}
									}
								]
							}
						]
					}
				]
			}
		}`)))

	// Create a test file
	err := GenerateOSV("testfile", true, 100000)
	defer os.Remove("testfile")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	createdFile, err := os.ReadFile("testfile")
	if err != nil {
		t.Fatalf("expected no error reading the file, got %v", err)
	}

	// Remove the modified field since it is set to current time
	re := regexp.MustCompile(`"modified":"[^"]*",`)
	result := re.ReplaceAllString(string(createdFile), "")
	// Replace randomly generated internal ID to a specific one for easier comparison
	re = regexp.MustCompile(`"_id":"[^"]*"`)
	result = re.ReplaceAllString(result, `"_id":"abcd"`)
	// Remove newlines
	result = strings.ReplaceAll(result, "\n", "")

	// Compare the results
	if result != sampleOSVResultContainers {
		t.Fatalf("expected different result: %v", cmp.Diff(string(sampleOSVResultContainers), string(result)))
	}

}
