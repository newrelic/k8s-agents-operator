package v1beta3

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/newrelic/k8s-agents-operator/internal/selector"
)

// NameSelectorOperator A name selector operator is the set of operators that can be used in a selector requirement.
type NameSelectorOperator string
type NameSelectorKey string

const (
	NameSelectorOpEquals    NameSelectorOperator = "=="
	NameSelectorOpNotEquals NameSelectorOperator = "!="
	NameSelectorOpIn        NameSelectorOperator = "In"
	NameSelectorOpNotIn     NameSelectorOperator = "NotIn"
)

const (
	NameSelectorKeyInitContainer NameSelectorKey = "initContainer"
	NameSelectorKeyContainer     NameSelectorKey = "container"
	NameSelectorKeyAnyContainer  NameSelectorKey = "anyContainer"
)

var acceptableNameSelectorOps = map[NameSelectorOperator]struct{}{
	NameSelectorOpEquals:    {},
	NameSelectorOpNotEquals: {},
	NameSelectorOpIn:        {},
	NameSelectorOpNotIn:     {},
}

var acceptableNameSelectorKeys = map[NameSelectorKey]struct{}{
	NameSelectorKeyInitContainer: {},
	NameSelectorKeyContainer:     {},
	NameSelectorKeyAnyContainer:  {},
}

type NameSelectorRequirement struct {
	// key is the field selector key that the requirement applies to.
	Key NameSelectorKey `json:"key"`
	// operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist.
	// The list of operators may grow in the future.
	Operator NameSelectorOperator `json:"operator"`
	// values is an array of string values.
	// If the operator is In or NotIn, the values array must be non-empty.
	// If the operator is Exists or DoesNotExist, the values array must be empty.
	// +optional
	// +listType=atomic
	Values []string `json:"values,omitempty"`
}

type NameSelector struct {
	// matchNames is a map of {key,value} pairs. A single {key,value} in the matchEnvs
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value". The requirements are ANDed.
	// +optional
	MatchNames map[string]string `json:"matchNames,omitempty"`
	// matchExpressions is a list of env selector requirements. The requirements are ANDed.
	// +optional
	// +listType=atomic
	MatchExpressions []NameSelectorRequirement `json:"matchExpressions,omitempty"`
}

// IsEmpty is used to check if the container selector is empty
func (s *NameSelector) IsEmpty() bool {
	return len(s.MatchNames) == 0 && len(s.MatchExpressions) == 0
}

func (s *NameSelector) AsSelector() (selector.Selector, error) {
	sel, err := selector.New(&selector.SimpleSelector{})
	if err != nil {
		return nil, err
	}
	if !s.IsEmpty() {
		lsr := make([]selector.SelectorRequirement, len(s.MatchExpressions))
		for i, entry := range s.MatchExpressions {
			lsr[i] = selector.SelectorRequirement{
				Key:      string(entry.Key),
				Operator: selector.SelectionOperator(entry.Operator),
				Values:   entry.Values,
			}
		}
		ls := selector.SimpleSelector{
			MatchExact:       s.MatchNames,
			MatchExpressions: lsr,
		}
		sel, err = selector.New(&ls, selector.WithKeyValidator(validateNameKey), selector.WithOperatorValidator(validateNameOperator))
		if err != nil {
			return nil, err
		}
	}
	return sel, nil
}

func validateNameKey(key string, opts *field.Path) *field.Error {
	if _, ok := acceptableNameSelectorKeys[NameSelectorKey(key)]; !ok {
		return field.Invalid(opts, key, "invalid key")
	}
	return nil
}

func validateNameOperator(operator string, opts *field.Path) *field.Error {
	if _, ok := acceptableNameSelectorOps[NameSelectorOperator(operator)]; !ok {
		return field.Invalid(opts, operator, "invalid operator")
	}
	return nil
}
