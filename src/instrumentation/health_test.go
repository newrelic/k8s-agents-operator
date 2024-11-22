package instrumentation

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"testing"
)

func TestSelectors(t *testing.T) {
	testCases := []struct {
		name               string
		podSelectors       metav1.LabelSelector
		namespaceSelectors metav1.LabelSelector
		pods               []*corev1.Pod
		namespaces         map[string]corev1.Namespace
		expectedCount      int
	}{
		{
			name: "1",
			podSelectors: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"bar"},
					},
				},
			},
			//namespaceSelectors: metav1.LabelSelector{},
			pods: []*corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default", Labels: map[string]string{"foo": "bar"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default", Labels: map[string]string{"foo": "eat"}}},
			},
			namespaces: map[string]corev1.Namespace{
				"default": corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{}},
				},
			},
			expectedCount: 1,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			c := 0

			podSelector, err := metav1.LabelSelectorAsSelector(&testCase.podSelectors)
			if err != nil {
				t.Fatal(err)
			}
			namespaceSelector, err := metav1.LabelSelectorAsSelector(&testCase.namespaceSelectors)
			if err != nil {
				t.Fatal(err)
			}

			for _, pod := range testCase.pods {
				ns, ok := testCase.namespaces[pod.Namespace]
				if !ok {
					t.Log("missing namespace")
					continue
				}
				if !namespaceSelector.Matches(labels.Set(ns.Labels)) {
					t.Log("namespace does not match selector")
					continue
				}
				if !podSelector.Matches(labels.Set(pod.Labels)) {
					t.Log("pod does not match selector")
					continue
				}
				c++
			}

			if c != testCase.expectedCount {
				t.Errorf("expected %d pods, got %d", testCase.expectedCount, c)
			}
		})
	}
}
