//go:build !ignore_autogenerated

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/newrelic/k8s-agents-operator/api/common"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Agent) DeepCopyInto(out *Agent) {
	*out = *in
	if in.VolumeSizeLimit != nil {
		in, out := &in.VolumeSizeLimit, &out.VolumeSizeLimit
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Agent.
func (in *Agent) DeepCopy() *Agent {
	if in == nil {
		return nil
	}
	out := new(Agent)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Exporter) DeepCopyInto(out *Exporter) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Exporter.
func (in *Exporter) DeepCopy() *Exporter {
	if in == nil {
		return nil
	}
	out := new(Exporter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HealthAgent) DeepCopyInto(out *HealthAgent) {
	*out = *in
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]v1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HealthAgent.
func (in *HealthAgent) DeepCopy() *HealthAgent {
	if in == nil {
		return nil
	}
	out := new(HealthAgent)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Instrumentation) DeepCopyInto(out *Instrumentation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Instrumentation.
func (in *Instrumentation) DeepCopy() *Instrumentation {
	if in == nil {
		return nil
	}
	out := new(Instrumentation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Instrumentation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationDefaulter) DeepCopyInto(out *InstrumentationDefaulter) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationDefaulter.
func (in *InstrumentationDefaulter) DeepCopy() *InstrumentationDefaulter {
	if in == nil {
		return nil
	}
	out := new(InstrumentationDefaulter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationList) DeepCopyInto(out *InstrumentationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Instrumentation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationList.
func (in *InstrumentationList) DeepCopy() *InstrumentationList {
	if in == nil {
		return nil
	}
	out := new(InstrumentationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InstrumentationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationSpec) DeepCopyInto(out *InstrumentationSpec) {
	*out = *in
	out.Exporter = in.Exporter
	in.Resource.DeepCopyInto(&out.Resource)
	if in.Propagators != nil {
		in, out := &in.Propagators, &out.Propagators
		*out = make([]common.Propagator, len(*in))
		copy(*out, *in)
	}
	out.Sampler = in.Sampler
	in.PodLabelSelector.DeepCopyInto(&out.PodLabelSelector)
	in.NamespaceLabelSelector.DeepCopyInto(&out.NamespaceLabelSelector)
	in.Agent.DeepCopyInto(&out.Agent)
	in.HealthAgent.DeepCopyInto(&out.HealthAgent)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationSpec.
func (in *InstrumentationSpec) DeepCopy() *InstrumentationSpec {
	if in == nil {
		return nil
	}
	out := new(InstrumentationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationStatus) DeepCopyInto(out *InstrumentationStatus) {
	*out = *in
	if in.UnhealthyPodsErrors != nil {
		in, out := &in.UnhealthyPodsErrors, &out.UnhealthyPodsErrors
		*out = make([]UnhealthyPodError, len(*in))
		copy(*out, *in)
	}
	in.LastUpdated.DeepCopyInto(&out.LastUpdated)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationStatus.
func (in *InstrumentationStatus) DeepCopy() *InstrumentationStatus {
	if in == nil {
		return nil
	}
	out := new(InstrumentationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationValidator) DeepCopyInto(out *InstrumentationValidator) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationValidator.
func (in *InstrumentationValidator) DeepCopy() *InstrumentationValidator {
	if in == nil {
		return nil
	}
	out := new(InstrumentationValidator)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Resource) DeepCopyInto(out *Resource) {
	*out = *in
	if in.Attributes != nil {
		in, out := &in.Attributes, &out.Attributes
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Resource.
func (in *Resource) DeepCopy() *Resource {
	if in == nil {
		return nil
	}
	out := new(Resource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Sampler) DeepCopyInto(out *Sampler) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Sampler.
func (in *Sampler) DeepCopy() *Sampler {
	if in == nil {
		return nil
	}
	out := new(Sampler)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UnhealthyPodError) DeepCopyInto(out *UnhealthyPodError) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UnhealthyPodError.
func (in *UnhealthyPodError) DeepCopy() *UnhealthyPodError {
	if in == nil {
		return nil
	}
	out := new(UnhealthyPodError)
	in.DeepCopyInto(out)
	return out
}
