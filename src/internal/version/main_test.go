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

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoInstrumentationJavaFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationJava())
}

func TestAutoInstrumentationNodeJSFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationNodeJS())
}

func TestAutoInstrumentationPythonFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationPython())
}

func TestAutoInstrumentationDotNetFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationDotNet())
}

func TestAutoInstrumentationPhpFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0.0", AutoInstrumentationPhp())
}

func TestAutoInstrumentationRubyFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationRuby())
}

func TestAutoInstrumentationGoFallbackVersion(t *testing.T) {
	assert.Equal(t, "0.0.0", AutoInstrumentationGo())
}
