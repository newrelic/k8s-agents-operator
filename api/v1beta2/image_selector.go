package v1beta2

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ImageSelectorOperator string
type ImageSelectorKey string

const (
	ImageSelectorOpIn    ImageSelectorOperator = "In"
	ImageSelectorOpNotIn ImageSelectorOperator = "NotIn"
)
const (
	ImageSelectorKeyRegistry  ImageSelectorKey = "registry"
	ImageSelectorKeyNamespace ImageSelectorKey = "namespace"
	ImageSelectorKeyName      ImageSelectorKey = "name"
	ImageSelectorKeyTag       ImageSelectorKey = "tag"
	ImageSelectorKeyDigest    ImageSelectorKey = "digest"
)

var acceptableImageSelectorOps = map[ImageSelectorOperator]struct{}{
	ImageSelectorOpIn:    {},
	ImageSelectorOpNotIn: {},
}

var acceptableImageSelectorKeys = map[ImageSelectorKey]struct{}{
	ImageSelectorKeyRegistry:  {},
	ImageSelectorKeyNamespace: {},
	ImageSelectorKeyName:      {},
	ImageSelectorKeyTag:       {},
	ImageSelectorKeyDigest:    {},
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

func (s *ImageSelector) AsSelector() (labels.Selector, error) {
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
			MatchLabels:      s.MatchImages,
			MatchExpressions: lsr,
		}
		selector, err = metav1.LabelSelectorAsSelector(&ls)
		if err != nil {
			return nil, err
		}
	}
	return selector, nil
}

func (s *ImageSelector) Validate() error {
	for _, exp := range s.MatchExpressions {
		if _, ok := acceptableImageSelectorKeys[ImageSelectorKey(exp.Key)]; !ok {
			return fmt.Errorf("invalid key selector key: %s", exp.Key)
		}
		if _, ok := acceptableImageSelectorOps[ImageSelectorOperator(exp.Operator)]; !ok {
			return fmt.Errorf("invalid key selector operator: %s", exp.Operator)
		}
	}
	for key := range s.MatchImages {
		if _, ok := acceptableImageSelectorKeys[ImageSelectorKey(key)]; !ok {
			return fmt.Errorf("invalid key selector key: %s", key)
		}
	}
	return nil
}
