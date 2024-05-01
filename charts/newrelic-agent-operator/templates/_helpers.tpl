{{/*
Expand the name of the chart.
*/}}
{{- define "newrelic-agent-operator.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "newrelic-agent-operator.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "newrelic-agent-operator.chart" -}}
{{- printf "%s" .Chart.Name | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "newrelic-agent-operator.labels" -}}
helm.sh/chart: {{ include "newrelic-agent-operator.chart" . }}
{{ include "newrelic-agent-operator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "newrelic-agent-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "newrelic-agent-operator.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "newrelic-agent-operator.serviceAccountName" -}}
{{- if .Values.controllerManager.manager.serviceAccount.create }}
{{- default (include "newrelic-agent-operator.name" .) .Values.controllerManager.manager.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controllerManager.manager.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the licenseKey
*/}}
{{- define "newrelic-agent-operator.licenseKey" -}}
{{- if .Values.global}}
  {{- if .Values.global.licenseKey }}
      {{- .Values.global.licenseKey -}}
  {{- else -}}
      {{- .Values.licenseKey | default "" -}}
  {{- end -}}
{{- else -}}
    {{- .Values.licenseKey | default "" -}}
{{- end -}}
{{- end -}}

{{/*
Returns if the template should render, it checks if the required values are set.
*/}}
{{- define "newrelic-agent-operator.areValuesValid" -}}
{{- $licenseKey := include "newrelic-agent-operator.licenseKey" . -}}
{{- and (or $licenseKey)}}
{{- end -}}

{{/*
Controller manager service certificate's secret.
*/}}
{{- define "newrelic-agent-operator.certificateSecret" -}}
{{- printf "%s-controller-manager-service-cert" (include "newrelic-agent-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}
