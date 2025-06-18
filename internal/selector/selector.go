package selector

import (
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"slices"
	"strconv"
	"strings"
)

type Selector interface {
	Matches(map[string]string) bool
}

type SelectionOperator string

const (
	selectionIn             SelectionOperator = "In"
	selectionNotIn          SelectionOperator = "NotIn"
	selectionEquals         SelectionOperator = "=="
	selectionNotEquals      SelectionOperator = "!="
	selectionExists         SelectionOperator = "Exists"
	selectionNotExists      SelectionOperator = "NotExists"
	selectionGreater        SelectionOperator = ">"
	selectionGreaterOrEqual SelectionOperator = ">="
	selectionLess           SelectionOperator = "<"
	selectionLessOrEqual    SelectionOperator = "<="
	selectionStartsWith     SelectionOperator = "StartsWith"
	selectionNotStartsWith  SelectionOperator = "NotStartsWith"
	selectionEndsWith       SelectionOperator = "EndsWith"
	selectionNotEndsWith    SelectionOperator = "NotEndsWith"
	selectionContains       SelectionOperator = "Contains"
	selectionNotContains    SelectionOperator = "NotContains"
)

var validRequirementOperators = []SelectionOperator{
	selectionIn, selectionNotIn,
	selectionEquals, selectionNotEquals,
	selectionExists, selectionNotExists,
	selectionGreater, selectionGreaterOrEqual,
	selectionLess, selectionLessOrEqual,
	selectionStartsWith, selectionEndsWith, selectionContains,
}

type SelectorRequirement struct {
	Key      string            `json:"key"`
	Operator SelectionOperator `json:"operator"`
	Values   []string          `json:"values,omitempty"`
}

type SimpleSelector struct {
	MatchExpressions []SelectorRequirement `json:"matchExpressions,omitempty"`
	MatchExact       map[string]string     `json:"matchExact,omitempty"`
}

var EverythingSelector = &everythingSelector{}

type everythingSelector struct{}

func (s everythingSelector) Matches(l map[string]string) bool {
	return true
}

func New(ps *SimpleSelector, opts ...requirementOption) (Selector, error) {
	if len(ps.MatchExact)+len(ps.MatchExpressions) == 0 {
		return EverythingSelector, nil
	}
	requirements := make([]Requirement, 0, len(ps.MatchExact)+len(ps.MatchExpressions))
	for k, v := range ps.MatchExact {
		r, err := newRequirement(k, selectionEquals, []string{v}, append(opts, WithPathOptions(field.WithPath(field.NewPath("matchExact", k))))...)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, *r)
	}
	for i, expr := range ps.MatchExpressions {
		r, err := newRequirement(expr.Key, expr.Operator, append([]string(nil), expr.Values...), append(opts, WithPathOptions(field.WithPath(field.NewPath("matchExpressions", strconv.Itoa(i)))))...)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, *r)
	}
	return Requirements(requirements), nil
}

type Requirements []Requirement

func (s Requirements) Add(reqs ...Requirement) Selector {
	return append(s, reqs...)
}

func (s Requirements) Matches(l map[string]string) bool {
	for i := range s {
		if matches := s[i].Matches(l); !matches {
			return false
		}
	}
	return true
}

type Requirement struct {
	Key      string
	Operator SelectionOperator
	Values   []string
}

type KeyValidator func(key string, opts *field.Path) *field.Error
type ValueValidator func(key, value string, opts *field.Path) *field.Error
type OperatorValidator func(operator string, opts *field.Path) *field.Error

func anyKeyValidator(key string, opts *field.Path) *field.Error           { return nil }
func anyValueValidator(key, value string, opts *field.Path) *field.Error  { return nil }
func anyOperatorValidator(operator string, opts *field.Path) *field.Error { return nil }

type RequirementOptions struct {
	fieldOpts        []field.PathOption
	validateKey      KeyValidator
	validateValue    ValueValidator
	validateOperator OperatorValidator
}

type requirementOption func(*RequirementOptions)

func WithPathOptions(pathOpts ...field.PathOption) requirementOption {
	return func(o *RequirementOptions) {
		o.fieldOpts = append(o.fieldOpts, pathOpts...)
	}
}

func WithKeyValidator(v KeyValidator) requirementOption {
	return func(o *RequirementOptions) {
		o.validateKey = v
	}
}

func WithValueValidator(v ValueValidator) requirementOption {
	return func(o *RequirementOptions) {
		o.validateValue = v
	}
}

func WithOperatorValidator(v OperatorValidator) requirementOption {
	return func(o *RequirementOptions) {
		o.validateOperator = v
	}
}

//nolint:gocyclo
func newRequirement(key string, op SelectionOperator, vals []string, opts ...requirementOption) (*Requirement, error) {
	var allErrs field.ErrorList
	o := RequirementOptions{
		validateKey:      anyKeyValidator,
		validateValue:    anyValueValidator,
		validateOperator: anyOperatorValidator,
	}
	for _, opt := range opts {
		opt(&o)
	}
	path := field.ToPath(o.fieldOpts...)
	if err := o.validateKey(key, path.Child("key")); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := o.validateOperator(string(op), path.Child("operator")); err != nil {
		allErrs = append(allErrs, err)
	}

	valuePath := path.Child("values")
	switch op {
	case selectionExists, selectionNotExists:
		if len(vals) != 0 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "values set must be empty for exists and does not exist"))
		}
	case selectionEquals, selectionNotEquals:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "exact-match compatibility requires one single value"))
		}
	case selectionStartsWith:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'startswith' operator, exactly one value is required"))
		}
	case selectionNotStartsWith:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'notstartswith' operator, exactly one value is required"))
		}
	case selectionEndsWith:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'endswith' operator, exactly one value is required"))
		}
	case selectionNotEndsWith:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'notendswith' operator, exactly one value is required"))
		}
	case selectionContains:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'contains' operator, exactly one value is required"))
		}
	case selectionNotContains:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'notcontains' operator, exactly one value is required"))
		}
	case selectionIn:
		if len(vals) == 0 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'in' operator, values set can't be empty"))
		}
	case selectionNotIn:
		if len(vals) == 0 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for 'notin' operator, values set can't be empty"))
		}
	case selectionGreater, selectionGreaterOrEqual, selectionLess, selectionLessOrEqual:
		if len(vals) != 1 {
			allErrs = append(allErrs, field.Invalid(valuePath, vals, "for '>=', '>', '<', '<=' operators, exactly one value is required"))
		}
		if _, err := strconv.ParseInt(vals[0], 10, 64); err != nil {
			allErrs = append(allErrs, field.Invalid(valuePath.Index(0), vals[0], "for '>=', '>', '<', '<=' operators, the value must be an integer"))
		}
	default:
		allErrs = append(allErrs, field.NotSupported(path.Child("operator"), op, validRequirementOperators))
	}
	for i := range vals {
		if err := o.validateValue(key, vals[i], valuePath.Index(i)); err != nil {
			allErrs = append(allErrs, err)
		}
	}
	return &Requirement{Key: key, Operator: op, Values: vals}, allErrs.ToAggregate()
}

//nolint:gocyclo
func (r *Requirement) Matches(ls map[string]string) bool {
	val, ok := ls[r.Key]
	switch r.Operator {
	case selectionIn, selectionEquals:
		if !ok {
			return false
		}
		return slices.Contains(r.Values, val)
	case selectionNotIn, selectionNotEquals:
		if !ok {
			return true
		}
		return !slices.Contains(r.Values, val)
	case selectionExists:
		return ok
	case selectionNotExists:
		return !ok
	case selectionStartsWith:
		if !ok {
			return false
		}
		return strings.HasPrefix(val, r.Values[0])
	case selectionNotStartsWith:
		if !ok {
			return false
		}
		return !strings.HasPrefix(val, r.Values[0])
	case selectionEndsWith:
		if !ok {
			return false
		}
		return strings.HasSuffix(val, r.Values[0])
	case selectionNotEndsWith:
		if !ok {
			return false
		}
		return !strings.HasSuffix(val, r.Values[0])
	case selectionContains:
		if !ok {
			return false
		}
		return strings.Contains(val, r.Values[0])
	case selectionNotContains:
		if !ok {
			return false
		}
		return !strings.Contains(val, r.Values[0])
	case selectionGreater, selectionGreaterOrEqual, selectionLess, selectionLessOrEqual:
		if !ok {
			return false
		}
		lsValue, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			klog.V(10).Infof("ParseInt failed for value %+v in field %+v, %+v", val, ls, err)
			return false
		}

		// There should be only one strValue in r.strValues, and can be converted to an integer.
		if len(r.Values) != 1 {
			klog.V(10).Infof("Invalid values count %+v of requirement %#v, for '>=', '>', '<', '<=' operators, exactly one value is required", len(r.Values), r)
			return false
		}

		var rValue int64
		rValue, err = strconv.ParseInt(r.Values[0], 10, 64)
		if err != nil {
			klog.V(10).Infof("ParseInt failed for value %+v in requirement %#v, for '>=', '>', '<', '<=' operators, the value must be an integer", r.Values[0], r)
			return false
		}
		return (r.Operator == selectionGreater && lsValue > rValue) ||
			(r.Operator == selectionGreaterOrEqual && lsValue >= rValue) ||
			(r.Operator == selectionLess && lsValue < rValue) ||
			(r.Operator == selectionLessOrEqual && lsValue <= rValue)
	default:
		return false
	}
}
