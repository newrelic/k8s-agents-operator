package v1beta2

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// EnvSelectorOperator A env selector operator is the set of operators that can be used in a selector requirement.
type EnvSelectorOperator string

const (
	EnvSelectorOpIn           EnvSelectorOperator = "In"
	EnvSelectorOpNotIn        EnvSelectorOperator = "NotIn"
	EnvSelectorOpExists       EnvSelectorOperator = "Exists"
	EnvSelectorOpDoesNotExist EnvSelectorOperator = "DoesNotExist"
)

var acceptableEnvSelectorOps = map[EnvSelectorOperator]struct{}{
	EnvSelectorOpIn:           {},
	EnvSelectorOpNotIn:        {},
	EnvSelectorOpExists:       {},
	EnvSelectorOpDoesNotExist: {},
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

func (s *EnvSelector) AsSelector() (labels.Selector, error) {
	var err error
	selector := labels.NewSelector()
	if !s.IsEmpty() {
		lsr := make([]metav1.LabelSelectorRequirement, len(s.MatchExpressions))
		for i, entry := range s.MatchExpressions {
			lsr[i] = metav1.LabelSelectorRequirement{
				Key:      string(entry.Key),
				Operator: metav1.LabelSelectorOperator(entry.Operator),
				Values:   entry.Values,
			}
		}
		ls := metav1.LabelSelector{
			MatchLabels:      s.MatchEnvs,
			MatchExpressions: lsr,
		}
		selector, err = metav1.LabelSelectorAsSelector(&ls)
		if err != nil {
			return nil, err
		}
	}
	return selector, nil
}

func (s *EnvSelector) Validate() error {
	for _, exp := range s.MatchExpressions {
		if _, ok := acceptableEnvSelectorOps[EnvSelectorOperator(exp.Operator)]; !ok {
			return fmt.Errorf("invalid key selector key: %s", exp.Key)
		}
	}
	return nil
}
