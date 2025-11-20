package v1beta3

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/newrelic/k8s-agents-operator/internal/selector"
)

type ImageSelectorOperator string
type ImageSelectorKey string

const (
	ImageSelectorOpEquals        ImageSelectorOperator = "=="
	ImageSelectorOpNotEquals     ImageSelectorOperator = "!="
	ImageSelectorOpIn            ImageSelectorOperator = "In"
	ImageSelectorOpNotIn         ImageSelectorOperator = "NotIn"
	ImageSelectorOpStartsWith    ImageSelectorOperator = "StartsWith"
	ImageSelectorOpNotStartsWith ImageSelectorOperator = "NotStartsWith"
	ImageSelectorOpEndsWith      ImageSelectorOperator = "EndsWith"
	ImageSelectorOpNotEndsWith   ImageSelectorOperator = "NotEndsWith"
	ImageSelectorOpContains      ImageSelectorOperator = "Contains"
	ImageSelectorOpNotContains   ImageSelectorOperator = "NotContains"
)
const (
	ImageSelectorKeyUrl ImageSelectorKey = "url"
)

var acceptableImageSelectorOps = map[ImageSelectorOperator]struct{}{
	ImageSelectorOpEquals:        {},
	ImageSelectorOpNotEquals:     {},
	ImageSelectorOpIn:            {},
	ImageSelectorOpNotIn:         {},
	ImageSelectorOpStartsWith:    {},
	ImageSelectorOpNotStartsWith: {},
	ImageSelectorOpEndsWith:      {},
	ImageSelectorOpNotEndsWith:   {},
	ImageSelectorOpContains:      {},
	ImageSelectorOpNotContains:   {},
}

var acceptableImageSelectorKeys = map[ImageSelectorKey]struct{}{
	ImageSelectorKeyUrl: {},
}

type ImageSelectorRequirement struct {
	// key is the field selector key that the requirement applies to.
	Key ImageSelectorKey `json:"key"`
	// operator represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist.
	// The list of operators may grow in the future.
	Operator ImageSelectorOperator `json:"operator"`
	// values is an array of string values.
	// If the operator is In or NotIn, the values array must be non-empty.
	// If the operator is Exists or DoesNotExist, the values array must be empty.
	// +optional
	// +listType=atomic
	Values []string `json:"values,omitempty"`
}

type ImageSelector struct {
	// matchImages is a map of {key,value} pairs. A single {key,value} in the matchImages
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value". The requirements are ANDed.
	// +optional
	MatchImages map[string]string `json:"matchImages,omitempty"`
	// matchExpressions is a list of env selector requirements. The requirements are ANDed.
	// +optional
	// +listType=atomic
	MatchExpressions []ImageSelectorRequirement `json:"matchExpressions,omitempty"`
}

// IsEmpty is used to check if the container selector is empty
func (s *ImageSelector) IsEmpty() bool {
	return len(s.MatchImages) == 0 && len(s.MatchExpressions) == 0
}

func (s *ImageSelector) AsSelector() (selector.Selector, error) {
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
			MatchExact:       s.MatchImages,
			MatchExpressions: lsr,
		}
		sel, err = selector.New(&ls, selector.WithKeyValidator(validateImageKey), selector.WithOperatorValidator(validateImageOperator))
		if err != nil {
			return nil, err
		}
	}
	return sel, nil
}

func validateImageKey(key string, opts *field.Path) *field.Error {
	if _, ok := acceptableImageSelectorKeys[ImageSelectorKey(key)]; !ok {
		return field.Invalid(opts, key, "invalid key")
	}
	return nil
}

func validateImageOperator(operator string, opts *field.Path) *field.Error {
	if _, ok := acceptableImageSelectorOps[ImageSelectorOperator(operator)]; !ok {
		return field.Invalid(opts, operator, "invalid operator")
	}
	return nil
}
