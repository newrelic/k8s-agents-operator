package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//func TestInstrumentationDefaultingWebhook(t *testing.T) {
//	inst := &Instrumentation{
//		ObjectMeta: metav1.ObjectMeta{
//			Annotations: map[string]string{
//				AnnotationDefaultAutoInstrumentationJava:   "java-img:1",
//				AnnotationDefaultAutoInstrumentationNodeJS: "nodejs-img:1",
//				AnnotationDefaultAutoInstrumentationPython: "python-img:1",
//				AnnotationDefaultAutoInstrumentationDotNet: "dotnet-img:1",
//			},
//		},
//	}
//	inst.Default()
//	assert.Equal(t, "java-img:1", inst.Spec.Java.Image)
//	assert.Equal(t, "nodejs-img:1", inst.Spec.NodeJS.Image)
//	assert.Equal(t, "python-img:1", inst.Spec.Python.Image)
//	assert.Equal(t, "dotnet-img:1", inst.Spec.DotNet.Image)
//}

func TestInstrumentationValidatingWebhook(t *testing.T) {
	tests := []struct {
		name string
		err  string
		inst Instrumentation
	}{
		{
			name: "argument is a number",
			inst: Instrumentation{
				Spec: InstrumentationSpec{
					Sampler: Sampler{
						Type:     ParentBasedTraceIDRatio,
						Argument: "0.99",
					},
				},
			},
		},
		{
			name: "argument is missing",
			inst: Instrumentation{
				Spec: InstrumentationSpec{
					Sampler: Sampler{
						Type: ParentBasedTraceIDRatio,
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err == "" {
				assert.Nil(t, test.inst.ValidateCreate())
				assert.Nil(t, test.inst.ValidateUpdate(nil))
			} else {
				err := test.inst.ValidateCreate()
				assert.Contains(t, err.Error(), test.err)
				err = test.inst.ValidateUpdate(nil)
				assert.Contains(t, err.Error(), test.err)
			}
		})
	}
}
