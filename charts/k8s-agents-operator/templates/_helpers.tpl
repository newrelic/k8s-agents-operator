{{/*
Returns if the template should render, it checks if the required values are set.
*/}}
{{- define "k8s-agents-operator.areValuesValid" -}}
{{- $licenseKey := include "newrelic.common.license._licenseKey" . -}}
{{- and (or $licenseKey)}}
{{- end -}}

{{- define "k8s-agents-operator.manager.image" -}}
{{- $managerRepository := .Values.controllerManager.manager.image.repository -}}
{{- $defaultRepository := "newrelic/k8s-agents-operator" -}}
{{- $registry := include "newrelic.common.images.registry" ( dict "imageRoot" (dict "repository" $defaultRepository) "defaultRegistry" "" "context" . ) -}}
{{- if and $registry (eq $managerRepository $defaultRepository) -}}
  {{- $managerRepository = printf "%s/%s" $registry $defaultRepository -}}
{{- end -}}
{{- $managerVersion := .Values.controllerManager.manager.image.version | default .Chart.AppVersion -}}
{{- if eq (substr 0 7 $managerVersion) "sha256:" -}}
{{- printf "%s@%s" $managerRepository $managerVersion -}}
{{- else -}}
{{- printf "%s:%s" $managerRepository $managerVersion -}}
{{- end -}}
{{- end -}}
