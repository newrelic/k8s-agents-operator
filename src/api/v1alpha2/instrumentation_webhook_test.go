package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
					Agent: Agent{Language: "java", Image: "java"},
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
					Agent: Agent{Language: "java", Image: "java"},
				},
			},
		},
	}
	for _, test := range tests {
		test.inst.Default()
		err := test.inst.ValidateCreate()
		require.NoError(t, err)
		t.Run(test.name, func(t *testing.T) {
			test.inst.Default()
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
