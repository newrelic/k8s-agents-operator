{{/*
Returns if the template should render, it checks if the required values are set.
*/}}
{{- define "k8s-agents-operator.areValuesValid" -}}
{{- $licenseKey := include "newrelic.common.license._licenseKey" . -}}
{{- and (or $licenseKey)}}
{{- end -}}

{{- define "k8s-agents-operator.manager.image" -}}
{{- $managerVersion := .Values.controllerManager.manager.image.version | default .Chart.AppVersion -}}
{{- if eq (substr 0 7 $managerVersion) "sha256:" -}}
{{- printf "%s@%s" .Values.controllerManager.manager.image.repository $managerVersion -}}
{{- else -}}
{{- printf "%s:%s" .Values.controllerManager.manager.image.repository $managerVersion -}}
{{- end -}}
{{- end -}}

{{/*
Validates admission webhook configuration values.
*/}}
{{- define "k8s-agents-operator.validateWebhookConfig" -}}
{{- /* Validate timeoutSeconds is within Kubernetes allowed range (1-30 seconds) */ -}}
{{- if hasKey .Values.admissionWebhooks "timeoutSeconds" }}
  {{- if ne (typeOf .Values.admissionWebhooks.timeoutSeconds) "<nil>" }}
    {{- $timeout := .Values.admissionWebhooks.timeoutSeconds | int }}
    {{- if or (lt $timeout 1) (gt $timeout 30) }}
      {{- fail "admissionWebhooks.timeoutSeconds must be between 1 and 30 seconds" }}
    {{- end }}
  {{- end }}
{{- end }}
{{- /* Validate failurePolicy is either Fail or Ignore */ -}}
{{- if not (or (eq .Values.admissionWebhooks.failurePolicy "Fail") (eq .Values.admissionWebhooks.failurePolicy "Ignore")) }}
  {{- fail "admissionWebhooks.failurePolicy must be either 'Fail' or 'Ignore'" }}
{{- end }}
{{- /* Validate podFailurePolicy is either Fail or Ignore */ -}}
{{- if not (or (eq .Values.admissionWebhooks.podFailurePolicy "Fail") (eq .Values.admissionWebhooks.podFailurePolicy "Ignore")) }}
  {{- fail "admissionWebhooks.podFailurePolicy must be either 'Fail' or 'Ignore'" }}
{{- end }}
{{- end -}}
