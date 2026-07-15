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

package tekton

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func writeStub(dir, name, script string) {
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755)
	Expect(err).NotTo(HaveOccurred())
}

func runSanitizer(scriptPath, logFile, mockDir string) (string, int) {
	cmd := exec.Command("sh", scriptPath)
	cmd.Env = []string{
		"LOG_FILE=" + logFile,
		"PATH=" + mockDir + ":/usr/bin:/bin",
		"HOME=/tmp",
	}
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return outBuf.String(), exitCode
}

var _ = Describe("log_sanitizer.sh", func() {
	var (
		tmpDir     string
		mockDir    string
		logFile    string
		scriptFile string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		mockDir = filepath.Join(tmpDir, "mocks")
		Expect(os.Mkdir(mockDir, 0755)).To(Succeed())
		logFile = filepath.Join(tmpDir, "renovate-logs.json")
		scriptFile = filepath.Join(tmpDir, "log_sanitizer.sh")
		Expect(os.WriteFile(scriptFile, []byte(logSanitizerScript), 0755)).To(Succeed())
	})

	When("log file does not exist", func() {
		It("should skip sanitization", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("Log file not found, skipping sanitization"))
		})
	})

	When("leaktk scan fails", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"some": "log"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `echo "scan error" >&2; exit 1`)
		})

		It("should call fail_safe and overwrite the log file", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("leaktk scan failed"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(ContainSubstring("Sanitization step failed"))
		})
	})

	When("leaktk returns empty stdout", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"some": "log"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `exit 0`)
		})

		It("should report no secrets detected", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("No secrets detected or scanner produced no output"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(Equal(`{"some": "log"}`))
		})
	})

	When("python3 parsing fails", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"some": "log"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `echo '{"results": [{"secret": "abc"}]}'`)
			writeStub(mockDir, "python3", `exit 1`)
		})

		It("should call fail_safe and overwrite the log file", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("Failed to parse leaktk output"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(ContainSubstring("Sanitization step failed"))
		})
	})

	When("leaktk returns results JSON but no secrets in it", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"some": "log"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `echo '{"results": []}'`)
			writeStub(mockDir, "python3", `echo ""`)
		})

		It("should report no secrets found", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("No secrets found in log file"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(Equal(`{"some": "log"}`))
		})
	})

	When("secrets are found", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"msg": "token my-secret-value here"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `echo '{"results": [{"secret": "my-secret-value"}]}'`)
			writeStub(mockDir, "python3", `echo "my-secret-value"`)
		})

		It("should redact the secret from the log file", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("Log sanitization complete"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(Equal(`{"msg": "token **REDACTED** here"}`))
			Expect(string(logContent)).NotTo(ContainSubstring("my-secret-value"))
		})
	})

	When("secrets contain sed-special characters", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(logFile, []byte(`{"msg": "token my.secret*value[0] here"}`), 0644)).To(Succeed())
			writeStub(mockDir, "leaktk", `echo '{"results": [{"secret": "my.secret*value[0]"}]}'`)
			writeStub(mockDir, "python3", `echo "my.secret*value[0]"`)
		})

		It("should correctly redact secrets with regex metacharacters", func() {
			output, exitCode := runSanitizer(scriptFile, logFile, mockDir)

			Expect(exitCode).To(Equal(0))
			Expect(output).To(ContainSubstring("Log sanitization complete"))
			logContent, err := os.ReadFile(logFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(Equal(`{"msg": "token **REDACTED** here"}`))
			Expect(string(logContent)).NotTo(ContainSubstring("my.secret*value[0]"))
		})
	})
})
