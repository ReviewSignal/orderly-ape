// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright (c) 2025 ReviewSignal
//

package worker

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkerProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Worker Provider Suite")
}

var localEnv *envtest.Environment
var localCfg *rest.Config

var workerEnv *envtest.Environment
var workerCfg *rest.Config

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var err error

	By("bootstrapping test environment (the 'local' cluster)")
	localEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		localEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	localCfg, err = localEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment (the 'worker' cluster)")
	workerEnv = &envtest.Environment{}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		workerEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	workerCfg, err = workerEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	// Prevent the metrics listener being created
	metricsserver.DefaultBindAddress = "0"
})

var _ = AfterSuite(func() {
	if localEnv != nil {
		Expect(localEnv.Stop()).To(Succeed())
	}

	if workerEnv != nil {
		Expect(workerEnv.Stop()).To(Succeed())
	}

	// Put the DefaultBindAddress back
	metricsserver.DefaultBindAddress = ":8080"
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
