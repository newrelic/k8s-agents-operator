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
package apm

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

const (
	volumeName        = "newrelic-instrumentation"
	initContainerName = "newrelic-instrumentation"
	sideCarName       = "opentelemetry-auto-instrumentation"

	// indicates whether newrelic agents should be injected or not.
	// Possible values are "true", "false" or "<Instrumentation>" name.
	annotationInjectJava                 = "instrumentation.newrelic.com/inject-java"
	annotationInjectJavaContainersName   = "instrumentation.newrelic.com/java-container-names"
	annotationInjectNodeJS               = "instrumentation.newrelic.com/inject-nodejs"
	annotationInjectNodeJSContainersName = "instrumentation.newrelic.com/nodejs-container-names"
	annotationInjectPython               = "instrumentation.newrelic.com/inject-python"
	annotationInjectPythonContainersName = "instrumentation.newrelic.com/python-container-names"
	annotationInjectDotNet               = "instrumentation.newrelic.com/inject-dotnet"
	annotationInjectDotnetContainersName = "instrumentation.newrelic.com/dotnet-container-names"
	annotationInjectPhp                  = "instrumentation.newrelic.com/inject-php"
	annotationInjectPhpContainersName    = "instrumentation.newrelic.com/php-container-names"
	annotationPhpExecCmd                 = "instrumentation.newrelic.com/php-exec-command"
	annotationInjectContainerName        = "instrumentation.newrelic.com/container-name"
	annotationInjectGo                   = "instrumentation.opentelemetry.io/inject-go"
	annotationGoExecPath                 = "instrumentation.opentelemetry.io/otel-go-auto-target-exe"
	annotationInjectGoContainerName      = "instrumentation.opentelemetry.io/go-container-name"
)

// Calculate if we already inject InitContainers.
func isInitContainerMissing(pod corev1.Pod) bool {
	for _, initContainer := range pod.Spec.InitContainers {
		if initContainer.Name == initContainerName {
			return false
		}
	}
	return true
}

func getIndexOfEnv(envs []corev1.EnvVar, name string) int {
	for i := range envs {
		if envs[i].Name == name {
			return i
		}
	}
	return -1
}

func validateContainerEnv(envs []corev1.EnvVar, envsToBeValidated ...string) error {
	for _, envToBeValidated := range envsToBeValidated {
		for _, containerEnv := range envs {
			if containerEnv.Name == envToBeValidated {
				if containerEnv.ValueFrom != nil {
					return fmt.Errorf("the container defines env var value via ValueFrom, envVar: %s", containerEnv.Name)
				}
				break
			}
		}
	}
	return nil
}
