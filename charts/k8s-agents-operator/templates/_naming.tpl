{{- /* Naming helpers*/ -}}
{{- define "k8s-agents-operator.deployment.fullname" -}}
{{- include "newrelic.common.naming.truncateToDNSWithSuffix" (dict "name" (include "newrelic.common.naming.fullname" .) "suffix" "deployment") -}}
{{- end -}}

{{- define "k8s-agents-operator.daemonset.fullname" -}}
{{- include "newrelic.common.naming.truncateToDNSWithSuffix" (dict "name" (include "newrelic.common.naming.fullname" .) "suffix" "daemonset") -}}
{{- end -}}

{{- define "k8s-agents-operator.deployment.configMap.fullname" -}}
{{- include "newrelic.common.naming.truncateToDNSWithSuffix" (dict "name" (include "newrelic.common.naming.fullname" .) "suffix" "deployment-config") -}}
{{- end -}}

{{- define "k8s-agents-operator.daemonset.configMap.fullname" -}}
{{- include "newrelic.common.naming.truncateToDNSWithSuffix" (dict "name" (include "newrelic.common.naming.fullname" .) "suffix" "daemonset-config") -}}
{{- end -}}

{{/* Controller manager service certificate's secret. */}}
{{- define "k8s-agents-operator.certificateSecret.fullname" -}}
{{- include "newrelic.common.naming.truncateToDNSWithSuffix" (dict "name" (include "newrelic.common.naming.fullname" .) "suffix" "controller-manager-service-cert") -}}
{{- end }}