package tools

import (
	// for setting up the test environment
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"

	// for generating manifests
	_ "sigs.k8s.io/kustomize/kustomize/v5"

	// for code formatting
	_ "golang.org/x/tools/cmd/goimports"

	// for code linting
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"

	// for generating CRDs
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"

	// for installing chart
	_ "helm.sh/helm/v4/cmd/helm"

	// for generating the helm chart from the manifests generated from kustomize
	_ "github.com/arttor/helmify/cmd/helmify"

	// for generating the helm docs from the template
	_ "github.com/norwoodj/helm-docs/cmd/helm-docs"

	// for running helm unit tests
	_ "github.com/helm-unittest/helm-unittest/cmd/helm-unittest"

	// for testing helm charts
	_ "github.com/helm/chart-testing/v3/ct"
)
