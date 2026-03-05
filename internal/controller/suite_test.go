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

package controller

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	//+kubebuilder:scaffold:imports
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	. "github.com/konflux-ci/mintmaker/internal/constant"
)

const (
	timeout  = time.Second * 2
	interval = time.Millisecond * 250
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	log       logr.Logger
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

// moduleDir returns the on-disk path for a Go module dependency, resolving the
// version from go.mod so it stays in sync automatically.
func moduleDir(modulePath string, subPath ...string) string {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		panic("could not read go.mod: " + err.Error())
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == modulePath {
			parts := append([]string{build.Default.GOPATH, "pkg", "mod", modulePath + "@" + fields[1]}, subPath...)
			return filepath.Join(parts...)
		}
	}
	panic("module not found in go.mod: " + modulePath)
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	log = ctrl.Log.WithName("testdebug")

	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../..", "config", "crd", "bases"),
			moduleDir("github.com/konflux-ci/application-api", "config", "crd", "bases"),
			moduleDir("github.com/tektoncd/pipeline", "config", "300-crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	cfg.Timeout = 5 * time.Second
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = appstudiov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = mmv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = tektonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		// Configure namespace-scoped cache to match production behavior.
		// This ensures tests validate the actual filtering mechanism.
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&mmv1alpha1.DependencyUpdateCheck{}: {
					Namespaces: map[string]cache.Config{
						MintMakerNamespaceName: {},
					},
				},
				&corev1.Event{}: {
					Namespaces: map[string]cache.Config{
						MintMakerNamespaceName: {},
					},
				},
				&tektonv1.PipelineRun{}: {
					Namespaces: map[string]cache.Config{
						MintMakerNamespaceName: {},
					},
				},
			},
		},
	})
	Expect(err).ToNot(HaveOccurred())

	err = (NewDependencyUpdateCheckReconciler(k8sManager.GetClient(), k8sManager.GetScheme(), k8sManager.GetEventRecorderFor("DependencyUpdateCheckController"))).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&PipelineRunReconciler{Client: k8sManager.GetClient(), Scheme: k8sManager.GetScheme()}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&EventReconciler{Client: k8sManager.GetClient(), Scheme: k8sManager.GetScheme()}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
