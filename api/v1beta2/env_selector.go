package v1beta2

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/newrelic/k8s-agents-operator/internal/selector"
)

// EnvSelectorOperator A env selector operator is the set of operators that can be used in a selector requirement.
type EnvSelectorOperator string

const (
	EnvSelectorOpIn           EnvSelectorOperator = "In"
	EnvSelectorOpNotIn        EnvSelectorOperator = "NotIn"
	EnvSelectorOpEquals       EnvSelectorOperator = "=="
	EnvSelectorOpNotEquals    EnvSelectorOperator = "!="
	EnvSelectorOpExists       EnvSelectorOperator = "Exists"
	EnvSelectorOpDoesNotExist EnvSelectorOperator = "DoesNotExist"
)

var acceptableEnvSelectorOps = map[EnvSelectorOperator]struct{}{
	EnvSelectorOpIn:           {},
	EnvSelectorOpNotIn:        {},
	EnvSelectorOpExists:       {},
	EnvSelectorOpDoesNotExist: {},
	EnvSelectorOpEquals:       {},
	EnvSelectorOpNotEquals:    {},
}

type EnvSelectorRequirement struct {
	// key is the field selector key that the requirement applies to.
	Key string `json:"key"`
	// operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist.
	// The list of operators may grow in the future.
	Operator EnvSelectorOperator `json:"operator"`
	// values is an array of string values.
	// If the operator is In or NotIn, the values array must be non-empty.
	// If the operator is Exists or DoesNotExist, the values array must be empty.
	// +optional
	// +listType=atomic
	Values []string `json:"values,omitempty"`
}

type EnvSelector struct {
	// matchEnvs is a map of {key,value} pairs. A single {key,value} in the matchEnvs
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value". The requirements are ANDed.
	// +optional
	MatchEnvs map[string]string `json:"matchEnvs,omitempty"`
	// matchExpressions is a list of env selector requirements. The requirements are ANDed.
	// +optional
	// +listType=atomic
	MatchExpressions []EnvSelectorRequirement `json:"matchExpressions,omitempty"`
}

// IsEmpty is used to check if the container selector is empty
func (s *EnvSelector) IsEmpty() bool {
	return len(s.MatchEnvs) == 0 && len(s.MatchExpressions) == 0
}

func (s *EnvSelector) AsSelector() (selector.Selector, error) {
	sel, err := selector.New(&selector.SimpleSelector{})
	if err != nil {
		return nil, err
	}
	if !s.IsEmpty() {
		lsr := make([]selector.SelectorRequirement, len(s.MatchExpressions))
		for i, entry := range s.MatchExpressions {
			lsr[i] = selector.SelectorRequirement{
				Key:      entry.Key,
				Operator: selector.SelectionOperator(entry.Operator),
				Values:   entry.Values,
			}
		}
		ls := selector.SimpleSelector{
			MatchExact:       s.MatchEnvs,
			MatchExpressions: lsr,
		}
		sel, err = selector.New(&ls, selector.WithOperatorValidator(validateEnvOperator))
		if err != nil {
			return nil, err
		}
	}
	return sel, nil
}

func validateEnvOperator(operator string, opts *field.Path) *field.Error {
	if _, ok := acceptableEnvSelectorOps[EnvSelectorOperator(operator)]; !ok {
		return field.Invalid(opts, operator, "invalid operator")
	}
	return nil
}
