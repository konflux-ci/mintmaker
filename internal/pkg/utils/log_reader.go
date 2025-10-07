// Copyright 2025 Red Hat, Inc.
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

package utils

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// necessary info of the failed Pod
type PodDetails struct {
	Name      string
	Namespace string
	TaskName  string
	Log       string // This will hold the error log(s)
}

// Renovate's numerical levels to standard string names
var renovateLogLevels = map[int]string{
	10: "TRACE",
	20: "DEBUG",
	30: "INFO",
	40: "WARN",
	50: "ERROR",
	60: "FATAL",
}

// Structured format for each log
type LogEntry struct {
	Timestamp time.Time
	Level     string // Human-readable log level (INFO, WARN, ERROR)
	Message   string
	Container string
	Pod       string
	Extras    map[string]any // Additional structured data
}

// Uses the controller-runtime client to inspect TaskRuns and find the failed Pod information. Uses the kubernetes.Clientset to retrieve failed Pod's logs.
func GetFailedPodDetails(ctx context.Context, client client.Client, k8sClientset *kubernetes.Clientset, pipelineRun *tektonv1.PipelineRun) (*PodDetails, error) {
	if pipelineRun.Status.ChildReferences == nil {
		return nil, fmt.Errorf("pipelineRun has no child references or status is incomplete")
	}

	for _, childRef := range pipelineRun.Status.ChildReferences {
		if childRef.Kind != "TaskRun" || childRef.APIVersion != tektonv1.SchemeGroupVersion.String() {
			continue
		}

		taskRun := &tektonv1.TaskRun{}
		taskRunKey := types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      childRef.Name,
		}

		if err := client.Get(ctx, taskRunKey, taskRun); err != nil {
			continue
		}

		taskCondition := taskRun.Status.GetCondition(apis.ConditionSucceeded)

		if taskCondition == nil || taskCondition.IsUnknown() || taskCondition.IsTrue() {
			continue
		}

		if taskRun.Status.PodName == "" {
			continue
		}

		structuredLogs, err := processLogStream(ctx, k8sClientset, taskRun.Status.PodName, pipelineRun.Namespace)
		var reason string
		if err != nil {
			ctrl.Log.WithName("LogReader").Error(err, "failed to process failed pod logs and retrieve detailed error messages")
			reason = taskCondition.Reason
		} else {
			reason = checkLogs(structuredLogs, taskCondition.Reason)
		}

		return &PodDetails{
			Name:      taskRun.Status.PodName,
			Namespace: pipelineRun.Namespace,
			TaskName:  getTaskRunTaskName(taskRun),
			Log:       reason,
		}, nil
	}

	return nil, fmt.Errorf("no failed TaskRun found with a valid PodName")
}

// helper function to safely retrieve task name
func getTaskRunTaskName(taskRun *tektonv1.TaskRun) string {
	if taskRun.Spec.TaskRef != nil {
		return taskRun.Spec.TaskRef.Name
	}
	return taskRun.Name
}

// Fetches logs from all containers in the Pod, attempts to parse JSON logs, and returns structured entries.
func processLogStream(ctx context.Context, clientset *kubernetes.Clientset, podName, namespace string) ([]LogEntry, error) {
	var structuredLogs []LogEntry

	// get the Pod
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Pod %s/%s: %v", namespace, podName, err)
	}

	// iterate and fetch logs for each container
	for _, container := range pod.Spec.Containers {
		logOptions := &corev1.PodLogOptions{
			Container: container.Name,
		}

		// create the log request and stream pod logs
		req := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOptions)
		podLogs, streamErr := req.Stream(ctx)
		if streamErr != nil {
			continue
		}

		// read the stream line by line
		scanner := bufio.NewScanner(podLogs)
		for scanner.Scan() {
			line := scanner.Text()

			// attempt to parse the JSON log line
			entry, err := parseLogLine(line, container.Name, podName)
			if err != nil {
				// log the non-JSON line as an UNKNOWN entry (only step-renovate has JSON logs)
				entry = LogEntry{
					Timestamp: time.Now(),
					Level:     "UNKNOWN",
					Message:   line,
					Container: container.Name,
					Pod:       podName,
				}
			}

			// append the structured entry
			structuredLogs = append(structuredLogs, entry)
		}
		podLogs.Close()
	}

	if len(structuredLogs) == 0 {
		return nil, fmt.Errorf("failed to read logs for pod %s/%s (checked %d containers)", namespace, podName, len(pod.Spec.Containers))
	}

	return structuredLogs, nil
}

// unmarshal the JSON log line and extract important fields
func parseLogLine(line, container, pod string) (LogEntry, error) {

	var rawData map[string]any
	if err := json.Unmarshal([]byte(line), &rawData); err != nil {
		return LogEntry{}, fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	// assign known fields to the final structure
	entry := LogEntry{
		Container: container,
		Pod:       pod,
		Extras:    make(map[string]any),
	}

	// extract standard fields, converting types as needed
	for k, v := range rawData {
		switch k {
		case "time":
			tsStr, ok := v.(string)
			if !ok {
				entry.Extras["time"] = v
				continue
			}

			ts, err := time.Parse(time.RFC3339, tsStr)
			if err != nil {
				entry.Extras["time"] = tsStr
				continue
			}

			entry.Timestamp = ts
		case "level":
			levelFloat, ok := v.(float64)
			if !ok {
				entry.Extras["level"] = v
				continue
			}

			levelInt := int(levelFloat)
			levelStr, found := renovateLogLevels[levelInt]
			if !found {
				entry.Level = fmt.Sprintf("LEVEL_%d", levelInt)
				continue
			}

			entry.Level = levelStr
		case "msg":
			msgStr, ok := v.(string)
			if !ok {
				entry.Extras["msg"] = v
				continue
			}

			entry.Message = msgStr
		case "name", "hostname", "pid", "logContext", "v":
			entry.Extras[k] = v
		default:
			entry.Extras[k] = v
		}
	}
	return entry, nil
}

// process structured logs to find errors/fatals and build a summary message
func checkLogs(logs []LogEntry, simpleReason string) string {
	errorsMap := make(map[string]int)
	fatalMap := make(map[string]int)

	// look only for ERROR and FATAL levels (which are the immediate cause of exit with non-zero code)
	// more detailed info will be shared through custom webhooks
	// iterate in reverse to get the most recent errors first
	for i := len(logs) - 1; i >= 0; i-- {
		logEntry := logs[i]
		switch logEntry.Level {
		case "FATAL":
			formattedErr := buildErrorMessage(logEntry)
			fatalMap[formattedErr]++
		case "ERROR":
			formattedErr := buildErrorMessage(logEntry)
			errorsMap[formattedErr]++
		}
	}

	// create summary for fatals with counts for duplicates
	errString := func(errors map[string]int) string {
		if len(errors) == 0 {
			return ""
		}

		totalCount := 0
		var uniqueMessages []string

		for msg, count := range errors {
			totalCount += count

			if count > 1 {
				uniqueMessages = append(uniqueMessages, fmt.Sprintf("%dx %s", count, msg))
			} else {
				uniqueMessages = append(uniqueMessages, msg)
			}
		}

		return fmt.Sprintf("%d ERROR:\n%s", totalCount, strings.Join(uniqueMessages, ""))
	}(errorsMap)

	// create summary for fatals with counts for duplicates
	fatalString := func(fatal map[string]int) string {
		if len(fatal) == 0 {
			return ""
		}

		totalCount := 0
		var uniqueMessages []string

		for msg, count := range fatal {
			totalCount += count

			if count > 1 {
				uniqueMessages = append(uniqueMessages, fmt.Sprintf("%dx %s", count, msg))
			} else {
				uniqueMessages = append(uniqueMessages, msg)
			}
		}

		return fmt.Sprintf("%d FATAL:\n%s", totalCount, strings.Join(uniqueMessages, ""))
	}(fatalMap)

	if errString == "" && fatalString == "" {
		errString = fmt.Sprintf("reason: %s", simpleReason)
	}

	container := logs[len(logs)-1].Container
	return fmt.Sprintf("Container %s exited with \n%s %s",
		container,
		errString,
		fatalString)
}

// build a single error message from a log entry, including nested error details if available
func buildErrorMessage(logEntry LogEntry) string {
	errMsg := logEntry.Message

	// Try to get additional error details
	if errMap, ok := logEntry.Extras["err"].(map[string]any); ok {
		if message, ok := errMap["message"].(string); ok {
			return fmt.Sprintf("%s: %s\n", errMsg, message)
		}
	}

	if errorMessage, ok := logEntry.Extras["errorMessage"].(string); ok {
		return fmt.Sprintf("%s: %s\n", errMsg, errorMessage)
	}

	return fmt.Sprintf("%s\n", errMsg)
}
