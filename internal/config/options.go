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

package config

import (
	"github.com/newrelic/k8s-agents-operator/internal/autodetect"
	"time"

	"github.com/go-logr/logr"

	"github.com/newrelic/k8s-agents-operator/internal/version"
)

// Option represents one specific configuration option.
type Option func(c *options)

type options struct {
	autoDetect              autodetect.AutoDetect
	version                 version.Version
	logger                  logr.Logger
	onOpenShiftRoutesChange changeHandler
	labelsFilter            []string
	openshiftRoutes         openshiftRoutesStore
	autoDetectFrequency     time.Duration
	autoscalingVersion      autodetect.AutoscalingVersion
}

func WithAutoDetect(a autodetect.AutoDetect) Option {
	return func(o *options) {
		o.autoDetect = a
	}
}
func WithAutoDetectFrequency(t time.Duration) Option {
	return func(o *options) {
		o.autoDetectFrequency = t
	}
}
func WithLogger(logger logr.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}
func WithOnOpenShiftRoutesChangeCallback(f func() error) Option {
	return func(o *options) {
		if o.onOpenShiftRoutesChange == nil {
			o.onOpenShiftRoutesChange = newOnChange()
		}
		o.onOpenShiftRoutesChange.Register(f)
	}
}
func WithPlatform(ora autodetect.OpenShiftRoutesAvailability) Option {
	return func(o *options) {
		o.openshiftRoutes.Set(ora)
	}
}
func WithVersion(v version.Version) Option {
	return func(o *options) {
		o.version = v
	}
}
