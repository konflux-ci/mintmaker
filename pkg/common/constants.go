package common

import "time"

const (
	MintMakerNamespaceName = "mintmaker"
	// Mintmaker will add processed annotation when the dependencyupdatecheck is processed by controller
	MintMakerProcessedAnnotationName = "mintmaker.appstudio.redhat.com/processed"
	// Mintmaker can be disabled by disabled annotation in component
	MintMakerDisabledAnnotationName = "mintmaker.appstudio.redhat.com/disabled"
	// Pipelines as Code GitHub appliaction configuration secret name.
	// The secret is located in Build Service namespace.
	PipelinesAsCodeGitHubAppSecretName = "pipelines-as-code-secret"
	// Keys of the GitHub app ID and the app private key in the pipelines-as-code-secret
	PipelinesAsCodeGithubAppIdKey   = "github-application-id"
	PipelinesAsCodeGithubPrivateKey = "github-private-key"
	GhTokenValidity                 = 1 * time.Hour
	GhTokenUsageWindow              = 30 * time.Minute
	GhTokenRenewThreshold           = GhTokenValidity - GhTokenUsageWindow
)
