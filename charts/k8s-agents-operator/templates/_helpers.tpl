{{/*
Expand the name of the chart.
*/}}
{{- define "k8s-agents-operator.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "k8s-agents-operator.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "k8s-agents-operator.chart" -}}
{{- printf "%s" .Chart.Name | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "k8s-agents-operator.labels" -}}
helm.sh/chart: {{ include "k8s-agents-operator.chart" . }}
{{ include "k8s-agents-operator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "k8s-agents-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-agents-operator.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "k8s-agents-operator.serviceAccountName" -}}
{{- if .Values.controllerManager.manager.serviceAccount.create }}
{{- default (include "k8s-agents-operator.name" .) .Values.controllerManager.manager.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controllerManager.manager.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the licenseKey
*/}}
{{- define "k8s-agents-operator.licenseKey" -}}
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
{{- define "k8s-agents-operator.areValuesValid" -}}
{{- $licenseKey := include "k8s-agents-operator.licenseKey" . -}}
{{- and (or $licenseKey)}}
{{- end -}}

{{/*
Controller manager service certificate's secret.
*/}}
{{- define "k8s-agents-operator.certificateSecret" -}}
{{- printf "%s-controller-manager-service-cert" (include "k8s-agents-operator.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Return certificate and CA for Webhooks.
It handles variants when a cert has to be generated by Helm,
a cert is loaded from an existing secret or is provided via `.Values`
*/}}
{{- define "k8s-agents-operator.webhookCert" -}}
{{- $caCert := "" }}
{{- $clientCert := "" }}
{{- $clientKey := "" }}
{{- if .Values.admissionWebhooks.autoGenerateCert.enabled }}
    {{- $prevSecret := (lookup "v1" "Secret" .Release.Namespace (include "k8s-agents-operator.certificateSecret" . )) }}
    {{- if and (not .Values.admissionWebhooks.autoGenerateCert.recreate) $prevSecret }}
        {{- $clientCert = index $prevSecret "data" "tls.crt" }}
        {{- $clientKey = index $prevSecret "data" "tls.key" }}
        {{- $caCert = index $prevSecret "data" "ca.crt" }}
        {{- if not $caCert }}
            {{- $prevHook := (lookup "admissionregistration.k8s.io/v1" "MutatingWebhookConfiguration" .Release.Namespace (print (include "k8s-agents-operator.fullname" . ) "-mutation")) }}
            {{- if not (eq (toString $prevHook) "<nil>") }}
                {{- $caCert = (first $prevHook.webhooks).clientConfig.caBundle }}
            {{- end }}
        {{- end }}
    {{- else }}
        {{- $certValidity := int .Values.admissionWebhooks.autoGenerateCert.certPeriodDays | default 365 }}
        {{- $ca := genCA "k8s-agents-operator-operator-ca" $certValidity }}
        {{- $domain1 := printf "%s-webhook-service.%s.svc" (include "k8s-agents-operator.fullname" .) $.Release.Namespace }}
        {{- $domain2 := printf "%s-webhook-service.%s.svc.%s" (include "k8s-agents-operator.fullname" .) $.Release.Namespace $.Values.kubernetesClusterDomain }}
        {{- $domains := list $domain1 $domain2 }}
        {{- $cert := genSignedCert (include "k8s-agents-operator.fullname" .) nil $domains $certValidity $ca }}
        {{- $clientCert = b64enc $cert.Cert }}
        {{- $clientKey = b64enc $cert.Key }}
        {{- $caCert = b64enc $ca.Cert }}
    {{- end }}
{{- else }}
    {{- $clientCert = .Files.Get .Values.admissionWebhooks.certFile | b64enc }}
    {{- $clientKey = .Files.Get .Values.admissionWebhooks.keyFile | b64enc }}
    {{- $caCert = .Files.Get .Values.admissionWebhooks.caFile | b64enc }}
{{- end }}
{{- $result := dict "clientCert" $clientCert "clientKey" $clientKey "caCert" $caCert }}
{{- $result | toYaml }}
{{- end }}
