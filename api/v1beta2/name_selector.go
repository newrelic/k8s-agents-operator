package v1beta2

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// NameSelectorOperator A name selector operator is the set of operators that can be used in a selector requirement.
type NameSelectorOperator string
type NameSelectorKey string

const (
	NameSelectorOpIn    NameSelectorOperator = "In"
	NameSelectorOpNotIn NameSelectorOperator = "NotIn"
)

const (
	NameSelectorKeyInitContainer NameSelectorKey = "initContainer"
	NameSelectorKeyContainer     NameSelectorKey = "container"
	NameSelectorKeyAnyContainer  NameSelectorKey = "anyContainer"
)

var acceptableNameSelectorOps = map[NameSelectorOperator]struct{}{
	NameSelectorOpIn:    {},
	NameSelectorOpNotIn: {},
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
	Operator EnvSelectorOperator `json:"operator"`
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

func (s *NameSelector) AsSelector() (labels.Selector, error) {
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
			MatchLabels:      s.MatchNames,
			MatchExpressions: lsr,
		}
		selector, err = metav1.LabelSelectorAsSelector(&ls)
		if err != nil {
			return nil, err
		}
	}
	return selector, nil
}

func (s *NameSelector) Validate() error {
	for _, exp := range s.MatchExpressions {
		if _, ok := acceptableNameSelectorKeys[NameSelectorKey(exp.Key)]; !ok {
			return fmt.Errorf("invalid key selector key: %s", exp.Key)
		}
		if _, ok := acceptableNameSelectorOps[NameSelectorOperator(exp.Operator)]; !ok {
			return fmt.Errorf("invalid key selector operator: %s", exp.Operator)
		}
	}
	for key := range s.MatchNames {
		if _, ok := acceptableNameSelectorKeys[NameSelectorKey(key)]; !ok {
			return fmt.Errorf("invalid key selector key: %s", key)
		}
	}
	return nil
}
