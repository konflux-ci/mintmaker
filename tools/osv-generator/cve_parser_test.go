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
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var vexSampleFile = []byte(`{
    "vulnerabilities": [{
        "cve": "CVE-2022-1234",
        "cwe": {
            "id": "CWE-79"
        },
        "discovery_date": "2022-01-01T00:00:00+00:00",
        "notes": [{
            "category": "summary",
            "text": "Test summary"
        }, {
            "category": "description",
            "text": "Test details"
        }],
        "references": [{
            "category": "self",
            "url": "http://example.com"
        }, {
			"category": "web",
            "url": "http://example2.com"
		}]
    }],
    "document": {
        "aggregate_severity": {
            "text": "High"
        }
    },
    "product_tree": {
        "branches": [{
            "branches": [{
                "category": "architecture",
                "branches": [{
                    "product": {
                        "product_identification_helper": {
                            "purl": "pkg:rpm/redhat/testpackage@1.0.1?arch=x86_64"
                        }
                    }
                }, {
                    "product": {
                        "product_identification_helper": {
                            "purl": "pkg:go/fakepackage@1.0.0?arch=x86_64"
                        }
                    }
                }, {
                    "product": {
                        "product_identification_helper": {
                            "purl": "pkg:oci/test-image@sha256:abcd?arch=amd64&repository_url=some-registry.com/org/repo&tag=v1"
                        }
                    }
                }]
            }, {
                "category": "irrelevant",
                "branches": [{
                    "product": {
                        "product_identification_helper": {
                            "purl": "pkg:go/fakepackage@9.9.9?arch=x86_64"
                        }
                    }
                }]
            }]
        }]
    }
}`)
var vexSampleObject VEX

// Initialize CSAF VEX object
func init() {
	if err := json.Unmarshal([]byte(vexSampleFile), &vexSampleObject); err != nil {
		panic(err)
	}
}

func TestGetAffectedListRPMs(t *testing.T) {
	affectedList := getAffectedListRPMs(vexSampleObject)

	if len(affectedList) != 1 {
		t.Fatalf("expected 1 affected package, got %d", len(affectedList))
	}
	if affectedList[0].Package.Name != "testpackage" {
		t.Fatalf("expected testpackage, got %s", affectedList[0].Package.Name)
	}
	if affectedList[0].Package.Purl != "pkg:rpm/redhat/testpackage" {
		t.Fatalf("expected pkg:rpm/redhat/testpackage, got %s", affectedList[0].Package.Purl)
	}
}

func TestGetAffectedListContainers(t *testing.T) {
	affectedList := getAffectedListContainers(vexSampleObject)

	if len(affectedList) != 1 {
		t.Fatalf("expected 1 affected package, got %d", len(affectedList))
	}
	if affectedList[0].Package.Name != "some-registry.com/org/repo" {
		t.Fatalf("expected some-registry.com/org/repo, got %s", affectedList[0].Package.Name)
	}
	if affectedList[0].Package.Purl != "pkg:oci/test-image@sha256:abcd?arch=amd64&repository_url=some-registry.com/org/repo&tag=v1" {
		t.Fatalf("pkg:oci/test-image@sha256:abcd?arch=amd64&repository_url=some-registry.com/org/repo&tag=v1, got %s", affectedList[0].Package.Purl)
	}
}

func TestGetReferencesList(t *testing.T) {
	references := getReferencesList(vexSampleObject.Vulnerabilities[0])
	if len(references) != 2 {
		t.Fatalf("expected 2 references, got %d", len(references))
	}

	if references[0].Type != "REPORT" || references[1].Type != "WEB" {
		t.Fatalf("unexpected reference types: %v", references)
	}
}

func TestGetDetails(t *testing.T) {
	details := getDetails(vexSampleObject.Vulnerabilities[0])
	if details != "Test details" {
		t.Fatalf("expected 'Test summary', got %s", details)
	}
}

func TestGetSummary(t *testing.T) {
	summary, ok := getSummary(vexSampleObject.Vulnerabilities[0])
	if !ok {
		t.Fatal("expected summary to be present")
	}
	if summary != "Test summary" {
		t.Fatalf("expected 'Test summary', got %s", summary)
	}
}

func TestGetSummaryMissing(t *testing.T) {
	vulnerability := &Vulnerability{
		Cve: "CVE-2022-9999",
		Notes: []struct {
			Category string `json:"category"`
			Text     string `json:"text"`
		}{
			{Category: "description", Text: "Some details"},
		},
	}
	summary, ok := getSummary(vulnerability)
	if ok {
		t.Fatal("expected summary to be absent")
	}
	if summary != "" {
		t.Fatalf("expected empty string, got %s", summary)
	}
}

func TestConvertToOSVSkipsVulnerabilityWithNoSummary(t *testing.T) {
	vex := VEX{}
	vex.Document.AggregateSeverity.Text = "High"
	vex.Vulnerabilities = []*Vulnerability{
		{
			Cve: "CVE-2022-9999",
			Notes: []struct {
				Category string `json:"category"`
				Text     string `json:"text"`
			}{
				{Category: "description", Text: "Some details"},
			},
		},
	}

	osv := ConvertToOSV(vex, false)
	if len(osv) != 0 {
		t.Fatalf("expected 0 OSVs (vulnerability should be skipped), got %d", len(osv))
	}
}

func TestGetPublishedDate(t *testing.T) {
	publishedDate := getPublishedDate(vexSampleObject.Vulnerabilities[0])
	expectedDate := "2022-01-01T00:00:00Z"
	if publishedDate != expectedDate {
		t.Fatalf("expected %s, got %s", expectedDate, publishedDate)
	}
}

func TestContains(t *testing.T) {
	affectedList := []*Affected{
		{
			Package: &Package{
				Name: "testpackage",
			},
		},
	}

	affectedPackage := Affected{
		Package: &Package{
			Name: "testpackage",
		},
	}

	if !contains(affectedList, affectedPackage) {
		t.Fatalf("expected package to be contained in the list")
	}

	affectedPackage.Package.Name = "anotherpackage"
	if contains(affectedList, affectedPackage) {
		t.Fatalf("expected package not to be contained in the list")
	}
}

func TestConvertToOSV(t *testing.T) {
	result := OSV{
		Id: "CVE-2022-1234",
		DatabaseSpecific: &DatabaseSpecific{
			Severity: "High",
			CWEids:   []string{"CWE-79"},
		},
	}

	osv := ConvertToOSV(vexSampleObject, false)
	if len(osv) != 1 {
		t.Fatalf("expected 1 OSV, got %d", len(osv))
	}
	if !cmp.Equal(osv[0].DatabaseSpecific, result.DatabaseSpecific) {
		t.Fatalf("expected %+v, got %+v", result.DatabaseSpecific, osv[0].DatabaseSpecific)
	}
	if !cmp.Equal(osv[0].Id, result.Id) {
		t.Fatalf("expected %+v, got %+v", result.Id, osv[0].Id)
	}
}
