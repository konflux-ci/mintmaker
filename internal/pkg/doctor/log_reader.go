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

package doctor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Name        string
	Namespace   string
	TaskName    string
	FailureLogs string
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
	Level  string // Human-readable log level (INFO, WARN, ERROR)
	Msg    string
	Extras map[string]any // Additional structured data
}

// Uses the controller-runtime client to inspect TaskRuns and find the failed Pod information. Uses the kubernetes.Clientset to retrieve failed Pod's logs.
func GetFailedPodDetails(ctx context.Context, client client.Client, Clientset *kubernetes.Clientset, pipelineRun *tektonv1.PipelineRun) (*PodDetails, error) {
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

		simpleReason := ""
		if !taskCondition.IsTrue() {
			simpleReason = taskCondition.Reason
		}

		reason, err := processLogStream(ctx, Clientset, taskRun.Status.PodName, pipelineRun.Namespace, simpleReason)

		if err != nil {
			ctrl.Log.WithName("LogReader").Error(err, "failed to process pod logs and retrieve detailed information")
		}

		return &PodDetails{
			Name:        taskRun.Status.PodName,
			Namespace:   pipelineRun.Namespace,
			TaskName:    getTaskRunTaskName(taskRun),
			FailureLogs: reason,
		}, nil
	}

	return nil, fmt.Errorf("no TaskRun found with a valid PodName")
}

// helper function to safely retrieve task name
func getTaskRunTaskName(taskRun *tektonv1.TaskRun) string {
	if taskRun.Spec.TaskRef != nil {
		return taskRun.Spec.TaskRef.Name
	}
	return taskRun.Name
}

// Fetches logs from all containers in the Pod, attempts to parse JSON logs, and returns structured entries.
func processLogStream(ctx context.Context, clientset *kubernetes.Clientset, podName, namespace, simpleReason string) (string, error) {
	containerRenovate := "step-renovate"
	errorsMap := make(map[string]int)
	fatalMap := make(map[string]int)
	failMsg := simpleReason

	// get the Pod
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return failMsg, fmt.Errorf("failed to get Pod %s/%s: %v", namespace, podName, err)
	}

	// iterate and fetch logs for each container
	for _, container := range pod.Spec.Containers {
		if container.Name != containerRenovate {
			continue
		}
		logOptions := &corev1.PodLogOptions{
			Container: container.Name,
		}

		// create the log request and stream pod logs
		req := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOptions)
		podLogs, streamErr := req.Stream(ctx)
		if streamErr != nil {
			continue
		}
		defer podLogs.Close()

		// read the stream line by line
		const maxBufferSize = 1 * 1024 * 1024 // bigger logs may need larger buffer
		scanner := bufio.NewScanner(podLogs)
		buf := make([]byte, maxBufferSize)
		scanner.Buffer(buf, maxBufferSize)
		for scanner.Scan() {
			line := string(scanner.Bytes())

			// attempt to parse the JSON log line
			entry, err := parseLogLine(line)
			if err == nil {
				switch entry.Level {
				case "FATAL":
					formattedErr := buildErrorMessage(entry)
					fatalMap[formattedErr]++
				case "ERROR":
					formattedErr := buildErrorMessage(entry)
					errorsMap[formattedErr]++
				}
			}
		}
	}

	failMsg = buildErrorMessageFromLogs(errorsMap, fatalMap, simpleReason)
	return failMsg, nil
}

// unmarshal the JSON log line and extract important fields
func parseLogLine(line string) (LogEntry, error) {

	var rawData map[string]any
	if err := json.Unmarshal([]byte(line), &rawData); err != nil {
		return LogEntry{}, fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	// assign known fields to the final structure
	entry := LogEntry{
		Extras: make(map[string]any),
	}

	// extract standard fields, converting types as needed
	for k, v := range rawData {
		switch k {
		// extract known Renovate log levels
		case "level":
			levelFloat, ok := v.(float64)
			if !ok {
				continue
			}

			levelInt := int(levelFloat)
			levelStr, found := renovateLogLevels[levelInt]
			if !found {
				continue
			}

			entry.Level = levelStr
		// keep a valid string log message
		case "msg":
			msgStr, ok := v.(string)
			if !ok {
				continue
			}

			entry.Msg = msgStr
		// keep only relevant extra fields
		case "err", "errorMessage":
			entry.Extras[k] = v
		}
	}
	return entry, nil
}

// process structured logs to find errors/fatals and build a summary message
func buildErrorMessageFromLogs(errorsMap, fatalMap map[string]int, simpleReason string) string {
	// create summary for fatals with counts for duplicates
	errString := formatFailMsg(errorsMap, "ERROR", simpleReason)

	// create summary for fatals with counts for duplicates
	fatalString := formatFailMsg(fatalMap, "FATAL", simpleReason)

	if errString == "" && fatalString == "" {
		errString = fmt.Sprintf("reason: %s", simpleReason)
	}

	if errString == fatalString {
		fatalString = ""
	}

	return fmt.Sprintf("Mintmaker failed with \n%s %s",
		errString,
		fatalString)
}

func formatFailMsg(logs map[string]int, logLevel, simpleReason string) string {
	if len(logs) == 0 {
		return simpleReason
	}

	totalCount := 0
	var uniqueMessages []string

	for msg, count := range logs {
		totalCount += count

		if count > 1 {
			uniqueMessages = append(uniqueMessages, fmt.Sprintf("%dx %s", count, msg))
		} else {
			uniqueMessages = append(uniqueMessages, msg)
		}
	}

	return fmt.Sprintf("%d %s:\n%s", totalCount, logLevel, strings.Join(uniqueMessages, ""))
}

// build a single error message from a log entry, including nested error details if available
func buildErrorMessage(logEntry LogEntry) string {
	errMsg := logEntry.Msg

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
