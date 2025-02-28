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
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

func TestUpgrade(t *testing.T) {
	ctx := context.Background()
	nsName := strings.ToLower(t.Name())
	err := k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	})
	require.NoError(t, err)

	inst := &current.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "newrelic-instrumentation",
			Namespace: nsName,
		},
	}
	defaulter := current.InstrumentationDefaulter{}
	_ = defaulter.Default(ctx, inst)
	err = k8sClient.Create(context.Background(), inst)
	require.NoError(t, err)

	up := &InstrumentationUpgrade{
		Logger: logr.Discard(),
		Client: k8sClient,
	}
	err = up.ManagedInstances(context.Background())
	require.NoError(t, err)

	updated := current.Instrumentation{}
	err = k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: nsName,
		Name:      "newrelic-instrumentation",
	}, &updated)
	require.NoError(t, err)
	// assert.Equal(t, "java:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationJava])
	// assert.Equal(t, "java:2", updated.Spec.Java.Image)
	// assert.Equal(t, "nodejs:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationNodeJS])
	// assert.Equal(t, "nodejs:2", updated.Spec.NodeJS.Image)
	// assert.Equal(t, "python:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationPython])
	// assert.Equal(t, "python:2", updated.Spec.Python.Image)
	// assert.Equal(t, "dotnet:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationDotNet])
	// assert.Equal(t, "dotnet:2", updated.Spec.DotNet.Image)
	// assert.Equal(t, "php:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationPhp])
	// assert.Equal(t, "php:2", updated.Spec.Php.Image)
	// assert.Equal(t, "ruby:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationRuby])
	// assert.Equal(t, "ruby:2", updated.Spec.Ruby.Image)
	// assert.Equal(t, "go:2", updated.Annotations[current.AnnotationDefaultAutoInstrumentationGo])
	// assert.Equal(t, "go:2", updated.Spec.Go.Image)
}
