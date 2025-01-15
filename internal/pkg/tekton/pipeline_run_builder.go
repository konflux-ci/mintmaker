/*
Copyright 2024.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package tekton

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unicode"

	"github.com/hashicorp/go-multierror"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type PipelineRunBuilder struct {
	err         *multierror.Error
	pipelineRun *tektonv1.PipelineRun
}

type MountOptions struct {
	// Which task to mount to. If empty, defaults to xxx (e.g., "renovate")
	TaskName string
	// Which steps to mount to. If empty, mounts to all steps
	StepNames []string
	// If the mount should be read-only. Default is true
	ReadOnly *bool
	// The permission mode for the mounted files
	DefaultMode *int32
	// Optional specifies whether the ConfigMap/Secret must exist
	Optional *bool
}

// NewMountOptions creates a new MountOptions with default values
func NewMountOptions() *MountOptions {
	readOnly := true
	defaultMode := int32(0644)
	optional := false

	return &MountOptions{
		TaskName:    "build",
		StepNames:   []string{},
		ReadOnly:    &readOnly,
		DefaultMode: &defaultMode,
		Optional:    &optional,
	}
}

func (o *MountOptions) WithTaskName(taskName string) *MountOptions {
	o.TaskName = taskName
	return o
}

func (o *MountOptions) WithStepNames(stepNames []string) *MountOptions {
	o.StepNames = stepNames
	return o
}

func (o *MountOptions) WithReadOnly(readOnly bool) *MountOptions {
	o.ReadOnly = &readOnly
	return o
}

func (o *MountOptions) WithDefaultMode(mode int32) *MountOptions {
	o.DefaultMode = &mode
	return o
}

func (o *MountOptions) WithOptional(optional bool) *MountOptions {
	o.Optional = &optional
	return o
}

// NewPipelineRunBuilder initializes a new PipelineRunBuilder with the given name prefix and namespace.
// It sets the name of the PipelineRun to be generated with the provided prefix and sets its namespace.
func NewPipelineRunBuilder(name, namespace string) *PipelineRunBuilder {
	return &PipelineRunBuilder{
		pipelineRun: &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: tektonv1.PipelineRunSpec{
				Status: tektonv1.PipelineRunSpecStatusPending,
				PipelineSpec: &tektonv1.PipelineSpec{
					Tasks: []tektonv1.PipelineTask{
						{
							Name: "build",
							TaskSpec: &tektonv1.EmbeddedTask{
								TaskSpec: tektonv1.TaskSpec{
									Steps: []tektonv1.Step{
										{
											Name:   "renovate",
											Image:  "quay.io/konflux-ci/mintmaker-renovate-image:latest",
											Script: `RENOVATE_TOKEN=$(cat /etc/renovate/secret/renovate-token) RENOVATE_CONFIG_FILE=/etc/renovate/config/renovate.json renovate`,
											SecurityContext: &corev1.SecurityContext{
												Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
												RunAsNonRoot:             ptr.To(true),
												AllowPrivilegeEscalation: ptr.To(false),
												SeccompProfile: &corev1.SeccompProfile{
													Type: corev1.SeccompProfileTypeRuntimeDefault,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Build returns the constructed PipelineRun and any accumulated error.
func (b *PipelineRunBuilder) Build() (*tektonv1.PipelineRun, error) {
	return b.pipelineRun, b.err.ErrorOrNil()
}

// WithAnnotations appends or updates annotations to the PipelineRun's metadata.
// If the PipelineRun does not have existing annotations, it initializes them before adding.
func (b *PipelineRunBuilder) WithAnnotations(annotations map[string]string) *PipelineRunBuilder {
	if b.pipelineRun.ObjectMeta.Annotations == nil {
		b.pipelineRun.ObjectMeta.Annotations = make(map[string]string)
	}
	for key, value := range annotations {
		b.pipelineRun.ObjectMeta.Annotations[key] = value
	}
	return b
}

// WithFinalizer adds the given finalizer to the PipelineRun's metadata.
func (b *PipelineRunBuilder) WithFinalizer(finalizer string) *PipelineRunBuilder {
	controllerutil.AddFinalizer(b.pipelineRun, finalizer)
	return b
}

// WithLabels appends or updates labels to the PipelineRun's metadata.
// If the PipelineRun does not have existing labels, it initializes them before adding.
func (b *PipelineRunBuilder) WithLabels(labels map[string]string) *PipelineRunBuilder {
	if b.pipelineRun.ObjectMeta.Labels == nil {
		b.pipelineRun.ObjectMeta.Labels = make(map[string]string)
	}
	for key, value := range labels {
		b.pipelineRun.ObjectMeta.Labels[key] = value
	}
	return b
}

// Mounts a ConfigMap to the specified task and steps.
// - name: ConfigMap name
// - mountPath: where the ConfigMap should be mounted to
// - items: items from ConfigMap to be mounted. If nil, all items will be mounted
// - opts: mount options
func (b *PipelineRunBuilder) WithConfigMap(name, mountPath string, items []corev1.KeyToPath, opts *MountOptions) *PipelineRunBuilder {
	if opts == nil {
		opts = &MountOptions{}
	}
	if opts.TaskName == "" {
		opts.TaskName = "build"
	}
	if opts.ReadOnly == nil {
		readOnly := true
		opts.ReadOnly = &readOnly
	}

	for i, task := range b.pipelineRun.Spec.PipelineSpec.Tasks {
		// Add volume when task matches
		if task.Name == opts.TaskName && task.TaskSpec != nil {
			volumeName := fmt.Sprintf("configmap-%s", name)
			volume := corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
						Items:       items,
						Optional:    opts.Optional,
						DefaultMode: opts.DefaultMode,
					},
				},
			}
			b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Volumes = append(
				b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Volumes,
				volume,
			)

			// Add volume mount to specified steps or all steps
			volumeMount := corev1.VolumeMount{
				Name:      volumeName,
				MountPath: mountPath,
				ReadOnly:  *opts.ReadOnly,
			}

			for j := range b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Steps {
				step := &b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Steps[j]
				if len(opts.StepNames) == 0 || slices.Contains(opts.StepNames, step.Name) {
					step.VolumeMounts = append(step.VolumeMounts, volumeMount)
				}
			}
			break
		}
	}
	// TODO: error when the specified task is not found
	return b
}

// Mounts a Secret to the specified task and steps.
// - name: Secret name
// - mountPath: where the Secret should be mounted to
// - items: items from Secret to be mounted. If nil, all items will be mounted
// - opts: mount options
func (b *PipelineRunBuilder) WithSecret(name, mountPath string, items []corev1.KeyToPath, opts *MountOptions) *PipelineRunBuilder {
	if opts == nil {
		opts = &MountOptions{}
	}
	if opts.TaskName == "" {
		opts.TaskName = "build"
	}
	if opts.ReadOnly == nil {
		readOnly := true
		opts.ReadOnly = &readOnly
	}

	// Find the specified task
	for i, task := range b.pipelineRun.Spec.PipelineSpec.Tasks {
		if task.Name == opts.TaskName && task.TaskSpec != nil {
			volumeName := fmt.Sprintf("secret-%s", name)
			volume := corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  name,
						Items:       items,
						Optional:    opts.Optional,
						DefaultMode: opts.DefaultMode,
					},
				},
			}
			b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Volumes = append(
				b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Volumes,
				volume,
			)

			// Add volume mount to specified steps or all steps
			volumeMount := corev1.VolumeMount{
				Name:      volumeName,
				MountPath: mountPath,
				ReadOnly:  *opts.ReadOnly,
			}

			for j := range b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Steps {
				step := &b.pipelineRun.Spec.PipelineSpec.Tasks[i].TaskSpec.TaskSpec.Steps[j]
				if len(opts.StepNames) == 0 || slices.Contains(opts.StepNames, step.Name) {
					step.VolumeMounts = append(step.VolumeMounts, volumeMount)
				}
			}
			break
		}
	}
	// TODO: error when the specified task is not found
	return b
}

// WithObjectReferences constructs tektonv1.Param entries for each of the provided client.Objects.
// Each param name is derived from the object's Kind (with the first letter made lowercase) and
// the value is a combination of the object's Namespace and Name.
func (b *PipelineRunBuilder) WithObjectReferences(objects ...client.Object) *PipelineRunBuilder {
	for _, obj := range objects {
		name := []rune(obj.GetObjectKind().GroupVersionKind().Kind)
		name[0] = unicode.ToLower(name[0])
		b.WithParams(tektonv1.Param{
			Name: string(name),
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: obj.GetNamespace() + "/" + obj.GetName(),
			},
		})
	}
	return b
}

// WithObjectSpecsAsJson constructs tektonv1.Param entries for the Spec field of each of the provided client.Objects.
// Each param name is derived from the object's Kind (with the first letter made lowercase).
// The value for each param is the JSON representation of the object's Spec.
// If an error occurs during extraction or serialization, it's accumulated in the builder's err field using multierror.
func (b *PipelineRunBuilder) WithObjectSpecsAsJson(objects ...client.Object) *PipelineRunBuilder {
	for _, obj := range objects {
		name := []rune(obj.GetObjectKind().GroupVersionKind().Kind)
		name[0] = unicode.ToLower(name[0])
		value := reflect.ValueOf(obj).Elem().FieldByName("Spec")
		if !value.IsValid() {
			b.err = multierror.Append(b.err, fmt.Errorf("failed to extract spec for object: %s", string(name)))
			continue
		}
		jsonData, err := json.Marshal(value.Interface())
		if err != nil {
			b.err = multierror.Append(b.err, fmt.Errorf("failed to serialize spec of object %s to JSON: %v", string(name), err))
			continue
		}
		b.WithParams(tektonv1.Param{
			Name: string(name),
			Value: tektonv1.ParamValue{
				Type:      tektonv1.ParamTypeString,
				StringVal: string(jsonData),
			},
		})
	}
	return b
}

// WithParams appends the provided params to the PipelineRun's spec.
func (b *PipelineRunBuilder) WithParams(params ...tektonv1.Param) *PipelineRunBuilder {
	if b.pipelineRun.Spec.Params == nil {
		b.pipelineRun.Spec.Params = make([]tektonv1.Param, 0)
	}
	b.pipelineRun.Spec.Params = append(b.pipelineRun.Spec.Params, params...)
	return b
}

// WithServiceAccount sets the ServiceAccountName for the PipelineRun's TaskRunTemplate.
func (b *PipelineRunBuilder) WithServiceAccount(serviceAccount string) *PipelineRunBuilder {
	b.pipelineRun.Spec.TaskRunTemplate.ServiceAccountName = serviceAccount
	return b
}

// WithTimeouts sets the Timeouts for the PipelineRun.
func (b *PipelineRunBuilder) WithTimeouts(timeouts, defaultTimeouts *tektonv1.TimeoutFields) *PipelineRunBuilder {
	if timeouts == nil || *timeouts == (tektonv1.TimeoutFields{}) {
		b.pipelineRun.Spec.Timeouts = defaultTimeouts
	} else {
		b.pipelineRun.Spec.Timeouts = timeouts
	}
	return b
}
