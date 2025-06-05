package util

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseContainerImage(t *testing.T) {
	tests := []struct {
		image       string
		expectedMap map[string]string
	}{
		{
			image:       "a",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "library", "tag": "latest"},
		}, {
			image:       "a:b",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "library", "tag": "b"},
		}, {
			image:       "a@sha256:c",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "library", "digest": "sha256:c"},
		}, {
			image:       "a:b@sha256:c",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "library", "tag": "b", "digest": "sha256:c"},
		}, {
			image:       "d/a",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "d", "tag": "latest"},
		}, {
			image:       "d/a:b",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "d", "tag": "b"},
		}, {
			image:       "d/a@sha256:c",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "d", "digest": "sha256:c"},
		}, {
			image:       "d/a:b@sha256:c",
			expectedMap: map[string]string{"name": "a", "registry": "docker.io", "namespace": "d", "tag": "b", "digest": "sha256:c"},
		}, {
			image:       "e/d/a",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e", "tag": "latest"},
		}, {
			image:       "e/d/a:b",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e", "tag": "b"},
		}, {
			image:       "e/d/a@sha256:c",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e", "digest": "sha256:c"},
		}, {
			image:       "e/d/a:b@sha256:c",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e", "tag": "b", "digest": "sha256:c"},
		}, {
			image:       "e:1/d/a",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e:1", "tag": "latest"},
		}, {
			image:       "e:1/d/a:b",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e:1", "tag": "b"},
		}, {
			image:       "e:1/d/a@sha256:c",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e:1", "digest": "sha256:c"},
		}, {
			image:       "e:1/d/a:b@sha256:c",
			expectedMap: map[string]string{"name": "a", "namespace": "d", "registry": "e:1", "tag": "b", "digest": "sha256:c"},
		}, {
			image:       "someimage",
			expectedMap: map[string]string{"name": "someimage", "namespace": "library", "registry": "docker.io", "tag": "latest"},
		}, {
			image:       "docker.io/someimage",
			expectedMap: map[string]string{"name": "someimage", "namespace": "library", "registry": "docker.io", "tag": "latest"},
		}, {
			image:       "localhost/someimage",
			expectedMap: map[string]string{"name": "someimage", "namespace": "library", "registry": "localhost", "tag": "latest"},
		}, {
			image:       "library/someimage",
			expectedMap: map[string]string{"name": "someimage", "namespace": "library", "registry": "docker.io", "tag": "latest"},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d_%s", i, test.image), func(t *testing.T) {
			if diff := cmp.Diff(test.expectedMap, ParseContainerImage(test.image)); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
