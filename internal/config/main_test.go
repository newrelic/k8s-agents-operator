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

package config_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newrelic/k8s-agents-operator/internal/autodetect"
	"github.com/newrelic/k8s-agents-operator/internal/config"
)

func TestNewConfig(t *testing.T) {
	// prepare
	cfg := config.New(
		config.WithPlatform(autodetect.OpenShiftRoutesNotAvailable),
	)

	// test
	assert.Equal(t, autodetect.OpenShiftRoutesNotAvailable, cfg.OpenShiftRoutes())
}

func TestOnPlatformChangeCallback(t *testing.T) {
	// prepare
	calledBack := false
	mock := &mockAutoDetect{
		OpenShiftRoutesAvailabilityFunc: func() (autodetect.OpenShiftRoutesAvailability, error) {
			return autodetect.OpenShiftRoutesAvailable, nil
		},
	}
	cfg := config.New(
		config.WithAutoDetect(mock),
		config.WithOnOpenShiftRoutesChangeCallback(func() error {
			calledBack = true
			return nil
		}),
	)

	// sanity check
	require.Equal(t, autodetect.OpenShiftRoutesNotAvailable, cfg.OpenShiftRoutes())

	// test
	err := cfg.AutoDetect()
	require.NoError(t, err)

	// verify
	assert.Equal(t, autodetect.OpenShiftRoutesAvailable, cfg.OpenShiftRoutes())
	assert.True(t, calledBack)
}

func TestAutoDetectInBackground(t *testing.T) {
	// prepare
	var ac int64
	tickTime := 100 * time.Millisecond
	mock := &mockAutoDetect{
		OpenShiftRoutesAvailabilityFunc: func() (autodetect.OpenShiftRoutesAvailability, error) {
			atomic.AddInt64(&ac, 1)
			return autodetect.OpenShiftRoutesNotAvailable, nil
		},
	}
	cfg := config.New(
		config.WithAutoDetect(mock),
		config.WithAutoDetectFrequency(tickTime),
	)

	// sanity check
	require.Equal(t, autodetect.OpenShiftRoutesNotAvailable, cfg.OpenShiftRoutes())

	// test
	err := cfg.StartAutoDetect()
	require.NoError(t, err)

	// verify
	time.Sleep(tickTime + 50*time.Millisecond)
	c := atomic.LoadInt64(&ac)
	assert.GreaterOrEqual(t, c, int64(2))
}

var _ autodetect.AutoDetect = (*mockAutoDetect)(nil)

type mockAutoDetect struct {
	OpenShiftRoutesAvailabilityFunc func() (autodetect.OpenShiftRoutesAvailability, error)
}

func (m *mockAutoDetect) HPAVersion() (autodetect.AutoscalingVersion, error) {
	return autodetect.DefaultAutoscalingVersion, nil
}

func (m *mockAutoDetect) OpenShiftRoutesAvailability() (autodetect.OpenShiftRoutesAvailability, error) {
	if m.OpenShiftRoutesAvailabilityFunc != nil {
		return m.OpenShiftRoutesAvailabilityFunc()
	}
	return autodetect.OpenShiftRoutesNotAvailable, nil
}
