/*
Copyright 2024.

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

package v1alpha2

//+kubebuilder:webhook:path=/mutate-newrelic-com-v1alpha2-instrumentation,mutating=true,failurePolicy=fail,sideEffects=None,groups=newrelic.com,resources=instrumentations,verbs=create;update,versions=v1alpha2,name=minstrumentation.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=create;update,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=fail,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationcreateupdate.kb.io,sideEffects=none,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=delete,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=ignore,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationdelete.kb.io,sideEffects=none,admissionReviewVersions=v1
