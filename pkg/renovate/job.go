package renovate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/konflux-ci/mintmaker/pkg/common"
)

const (
	TasksPerJob                = 20
	InstallationsPerJobEnvName = "RENOVATE_INSTALLATIONS_PER_JOB"
	TimeToLiveOfJob            = 24 * time.Hour
	RenovateImageEnvName       = "RENOVATE_IMAGE"
	DefaultRenovateImageUrl    = "quay.io/konflux-ci/mintmaker-renovate-image:latest"
)

// JobCoordinator is responsible for creating and managing renovate k8s jobs
type JobCoordinator struct {
	tasksPerJob      int
	renovateImageUrl string
	debug            bool
	client           client.Client
	scheme           *runtime.Scheme
}

func NewJobCoordinator(client client.Client, scheme *runtime.Scheme) *JobCoordinator {
	var tasksPerJobInt int
	tasksPerJobStr := os.Getenv(InstallationsPerJobEnvName)
	if regexp.MustCompile(`^\d{1,2}$`).MatchString(tasksPerJobStr) {
		tasksPerJobInt, _ = strconv.Atoi(tasksPerJobStr)
		if tasksPerJobInt == 0 {
			tasksPerJobInt = TasksPerJob
		}
	} else {
		tasksPerJobInt = TasksPerJob
	}
	renovateImageUrl := os.Getenv(RenovateImageEnvName)
	if renovateImageUrl == "" {
		renovateImageUrl = DefaultRenovateImageUrl
	}
	return &JobCoordinator{tasksPerJob: tasksPerJobInt, renovateImageUrl: renovateImageUrl, client: client, scheme: scheme, debug: true}
}

// getCAConfigMap returns the first ConfigMap found in mintmaker namespace
// that has the label 'config.openshift.io/inject-trusted-cabundle: "true"'.
// If no such ConfigMap is found, it returns nil.
func (j *JobCoordinator) getCAConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	configMapList := &corev1.ConfigMapList{}
	labelSelector := client.MatchingLabels{"config.openshift.io/inject-trusted-cabundle": "true"}
	listOptions := []client.ListOption{
		client.InNamespace(MintMakerNamespaceName),
		labelSelector,
	}
	err := j.client.List(ctx, configMapList, listOptions...)
	if err != nil {
		return nil, err
	}

	if len(configMapList.Items) > 0 {
		// Just return the configmap
		return &configMapList.Items[0], nil
	}

	return nil, nil
}

// Create a secret that merges all secret with the label:
// mintmaker.appstudio.redhat.com/secret-type: registry
// and return the new secret
func (j *JobCoordinator) createMergedPullSecret(ctx context.Context) (*corev1.Secret, error) {
	log := logger.FromContext(ctx)

	secretList := &corev1.SecretList{}
	labelSelector := client.MatchingLabels{"mintmaker.appstudio.redhat.com/secret-type": "registry"}
	listOptions := []client.ListOption{
		client.InNamespace(MintMakerNamespaceName),
		labelSelector,
	}

	err := j.client.List(ctx, secretList, listOptions...)

	if err != nil {
		return nil, err
	}

	if len(secretList.Items) == 0 {
		// No secrets to merge
		return nil, nil
	}

	log.Info(fmt.Sprintf("Found %d secrets to merge", len(secretList.Items)))

	mergedAuths := make(map[string]interface{})
	for _, secret := range secretList.Items {
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			data, exists := secret.Data[".dockerconfigjson"]
			if !exists {
				// No .dockerconfigjson section
				log.Info("Found secret without .dockerconfigjson section")
				return nil, nil
			}

			var dockerConfig map[string]interface{}
			if err := json.Unmarshal(data, &dockerConfig); err != nil {
				return nil, err
			}

			auths, exists := dockerConfig["auths"].(map[string]interface{})
			if !exists {
				continue
			}

			for registry, creds := range auths {
				mergedAuths[registry] = creds
			}
		}
	}

	mergedDockerConfig := map[string]interface{}{
		"auths": mergedAuths,
	}

	if len(mergedAuths) == 0 {
		log.Info("Merged auths empty, skipping creation of secret")
		return nil, nil
	}

	mergedConfigJson, err := json.Marshal(mergedDockerConfig)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("renovate-image-pull-secrets-%d-%s", timestamp, RandomString(5))

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(mergedConfigJson),
		},
	}

	if err := j.client.Create(ctx, newSecret); err != nil {
		return nil, err
	}

	return newSecret, nil
}

func (j *JobCoordinator) Execute(ctx context.Context, tasks []*Task) error {

	if len(tasks) == 0 {
		return nil
	}
	log := logger.FromContext(ctx)

	timestamp := time.Now().Unix()
	name := fmt.Sprintf("renovate-job-%d-%s", timestamp, RandomString(5))
	log.Info(fmt.Sprintf("Creating renovate job %s for %d unique sets of scm repositories", name, len(tasks)))

	secretTokens := map[string]string{}
	configMapData := map[string]string{}
	var renovateCmd []string

	// Create a merged secret for private registries
	registry_secret, err := j.createMergedPullSecret(ctx)

	for _, task := range tasks {
		taskId := RandomString(5)
		secretTokens[taskId] = task.Token

		config, err := task.GetJobConfig(ctx, j.client, registry_secret)
		if err != nil {
			return err
		}
		configMapData[fmt.Sprintf("%s.json", taskId)] = config

		renovateCmd = append(renovateCmd,
			fmt.Sprintf("RENOVATE_TOKEN=$TOKEN_%s RENOVATE_CONFIG_FILE=/configs/%s.json renovate || true", taskId, taskId),
		)
	}
	if len(renovateCmd) == 0 {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		StringData: secretTokens,
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Data: configMapData,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(TimeToLiveOfJob.Seconds())),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: name,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "renovate",
							Image: j.renovateImageUrl,
							Env: []corev1.EnvVar{
								{
									Name:  "HOME",
									Value: "/home/renovate",
								},
								{
									Name:  "RENOVATE_PR_FOOTER",
									Value: "To execute skipped test pipelines write comment `/ok-to-test`.\n\nThis PR has been generated by [MintMaker](https://github.com/konflux-ci/mintmaker) (powered by [Renovate Bot](https://github.com/renovatebot/renovate)).",
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									Prefix: "TOKEN_",
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: name,
										},
									},
								},
							},
							Command: []string{"bash", "-c", strings.Join(renovateCmd, "; ")},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      name,
									MountPath: "/configs",
								},
							},
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
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if j.debug {
		job.Spec.Template.Spec.Containers[0].Env = append(job.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"})
	}
	if err := j.client.Create(ctx, secret); err != nil {
		return err
	}
	if err := j.client.Create(ctx, configMap); err != nil {
		return err
	}

	// Check if a ConfigMap with the label `config.openshift.io/inject-trusted-cabundle: "true"` exists.
	// If such a ConfigMap is found, add a volume to the job specification to mount this ConfigMap.
	// The volume will be mounted at '/etc/pki/ca-trust/extracted/pem' within the job Pod.
	caConfigMap, err := j.getCAConfigMap(ctx)
	if err != nil {
		return err
	}
	if caConfigMap != nil {
		caVolume := corev1.Volume{
			Name: "trusted-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: caConfigMap.Name},
					Items: []corev1.KeyToPath{
						{
							Key:  "ca-bundle.crt",
							Path: "tls-ca-bundle.pem",
						},
					},
				},
			},
		}
		caVolumeMount := corev1.VolumeMount{
			Name:      "trusted-ca",
			MountPath: "/etc/pki/ca-trust/extracted/pem",
			ReadOnly:  true,
		}
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, caVolume)
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, caVolumeMount)
	}

	if registry_secret != nil {
		registrySecretVolume := corev1.Volume{
			Name: "registry-secrets",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: registry_secret.Name,
					Items: []corev1.KeyToPath{
						{
							Key:  ".dockerconfigjson",
							Path: "config.json",
						},
					},
				},
			},
		}
		registrySecretMount := corev1.VolumeMount{
			Name:      "registry-secrets",
			MountPath: "/home/renovate/.docker",
			ReadOnly:  true,
		}
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, registrySecretVolume)
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, registrySecretMount)
	}

	// Create the job
	if err := j.client.Create(ctx, job); err != nil {
		return err
	}
	log.Info("renovate job created", "jobname", job.Name, "tasks", len(tasks))

	// Set ownership so all resources get deleted once the job is deleted
	if err := controllerutil.SetOwnerReference(job, secret, j.scheme); err != nil {
		return err
	}
	if err := j.client.Update(ctx, secret); err != nil {
		return err
	}

	if registry_secret != nil {
		if err := controllerutil.SetOwnerReference(job, registry_secret, j.scheme); err != nil {
			return err
		}
		if err := j.client.Update(ctx, registry_secret); err != nil {
			return err
		}
	}

	if err := controllerutil.SetOwnerReference(job, configMap, j.scheme); err != nil {
		return err
	}
	if err := j.client.Update(ctx, configMap); err != nil {
		return err
	}
	return nil
}

func (j *JobCoordinator) ExecuteWithLimits(ctx context.Context, tasks []*Task) error {

	for i := 0; i < len(tasks); i += j.tasksPerJob {
		end := i + j.tasksPerJob

		if end > len(tasks) {
			end = len(tasks)
		}
		err := j.Execute(ctx, tasks[i:end])
		if err != nil {
			return err
		}
	}
	return nil

}
