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
package upgrade

import (
	"context"
	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/instrumentation"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

func TestUpgrade(t *testing.T) {
	logger := logr.Discard()
	ctx := context.Background()
	nsName := strings.ToLower(t.Name())
	err := k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	})
	require.NoError(t, err)

	inst := &v1alpha2.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "newrelic-instrumentation",
			Namespace: nsName,
		},
	}
	defaulter := instrumentation.InstrumentationDefaulter{Logger: logger}
	_ = defaulter.Default(ctx, inst)
	err = k8sClient.Create(context.Background(), inst)
	require.NoError(t, err)

	up := &InstrumentationUpgrade{
		Logger: logr.Discard(),
		Client: k8sClient,
	}
	err = up.ManagedInstances(context.Background())
	require.NoError(t, err)

	updated := v1alpha2.Instrumentation{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: nsName,
		Name:      "newrelic-instrumentation",
	}, &updated)
	require.NoError(t, err)
	// assert.Equal(t, "java:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationJava])
	// assert.Equal(t, "java:2", updated.Spec.Java.Image)
	// assert.Equal(t, "nodejs:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationNodeJS])
	// assert.Equal(t, "nodejs:2", updated.Spec.NodeJS.Image)
	// assert.Equal(t, "python:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationPython])
	// assert.Equal(t, "python:2", updated.Spec.Python.Image)
	// assert.Equal(t, "dotnet:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationDotNet])
	// assert.Equal(t, "dotnet:2", updated.Spec.DotNet.Image)
	// assert.Equal(t, "php:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationPhp])
	// assert.Equal(t, "php:2", updated.Spec.Php.Image)
	// assert.Equal(t, "ruby:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationRuby])
	// assert.Equal(t, "ruby:2", updated.Spec.Ruby.Image)
	// assert.Equal(t, "go:2", updated.Annotations[v1alpha2.AnnotationDefaultAutoInstrumentationGo])
	// assert.Equal(t, "go:2", updated.Spec.Go.Image)
}
