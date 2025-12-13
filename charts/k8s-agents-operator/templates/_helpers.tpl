{{/*
================================================================================
Helm Template Helpers for k8s-agents-operator
================================================================================
This file contains reusable template helpers for the k8s-agents-operator chart.

These helpers follow the common-library pattern but are extended to support
SHA digest-based image references (e.g., @sha256:...) in addition to tags.
*/}}

{{/*
================================================================================
Validation Helpers
================================================================================
*/}}

{{/*
Validates that required values are set before rendering templates.

Usage:
  {{- if include "k8s-agents-operator.areValuesValid" . -}}
  ...
  {{- end -}}

Returns: "true" if all required values are present, empty string otherwise
*/}}
{{- define "k8s-agents-operator.areValuesValid" -}}
  {{- $licenseKey := include "newrelic.common.license._licenseKey" . -}}
  {{- and (or $licenseKey) -}}
{{- end -}}

{{/*
================================================================================
Image Helpers
================================================================================
These helpers follow the common-library pattern (dict-based parameters) but
extend functionality to support SHA digest references.
*/}}

{{/*
Returns the proper image registry for the operator manager.

Follows the common-library 3-tier fallback pattern:
  1. .imageRoot.registry (local/component-specific)
  2. .context.Values.global.images.registry (global default)
  3. Empty string (use default Docker Hub)

Usage:
  {{- include "k8s-agents-operator.images.registry" ( dict "imageRoot" .Values.controllerManager.manager.image "context" . ) -}}

Returns: Registry hostname or empty string
*/}}
{{- define "k8s-agents-operator.images.registry" -}}
  {{- $globalRegistry := "" -}}
  {{- if .context.Values.global -}}
    {{- if .context.Values.global.images -}}
      {{- $globalRegistry = .context.Values.global.images.registry | default "" -}}
    {{- end -}}
  {{- end -}}

  {{- $localRegistry := "" -}}
  {{- if .imageRoot.registry -}}
    {{- $localRegistry = .imageRoot.registry -}}
  {{- end -}}

  {{- $localRegistry | default $globalRegistry | default "" -}}
{{- end -}}

{{/*
Returns the proper image repository for the operator manager.

Usage:
  {{- include "k8s-agents-operator.images.repository" .Values.controllerManager.manager.image -}}

Returns: Repository name (e.g., "newrelic/k8s-agents-operator")
*/}}
{{- define "k8s-agents-operator.images.repository" -}}
  {{- .repository | default "newrelic/k8s-agents-operator" -}}
{{- end -}}

{{/*
Returns the proper image version/tag for the operator manager.

Supports both semantic version tags and SHA digest references.

Usage:
  {{- include "k8s-agents-operator.images.version" ( dict "imageRoot" .Values.controllerManager.manager.image "context" . ) -}}

Returns: Version tag or SHA digest (e.g., "v1.0.0" or "sha256:abc...")
*/}}
{{- define "k8s-agents-operator.images.version" -}}
  {{- .imageRoot.version | default .context.Chart.AppVersion | toString -}}
{{- end -}}

{{/*
Constructs the complete container image reference for the operator manager.

This helper extends the common-library pattern to properly handle SHA digest
references using the @ separator instead of : (e.g., repo@sha256:... instead
of repo:sha256:...).

Resolution order for registry:
  1. .Values.controllerManager.manager.image.registry (component-specific)
  2. .Values.global.images.registry (global default)
  3. Empty (use default Docker Hub)

Resolution order for repository:
  1. .Values.controllerManager.manager.image.repository
  2. "newrelic/k8s-agents-operator" (default)

Resolution order for version:
  1. .Values.controllerManager.manager.image.version
  2. .Chart.AppVersion

Usage:
  image: {{ include "k8s-agents-operator.manager.image" . }}

Returns: Complete image reference (e.g., "registry.io/repo:tag" or "registry.io/repo@sha256:...")
*/}}
{{- define "k8s-agents-operator.manager.image" -}}
  {{- $imageRoot := .Values.controllerManager.manager.image -}}
  {{- $registry := include "k8s-agents-operator.images.registry" ( dict "imageRoot" $imageRoot "context" . ) -}}
  {{- $repository := include "k8s-agents-operator.images.repository" $imageRoot -}}
  {{- $version := include "k8s-agents-operator.images.version" ( dict "imageRoot" $imageRoot "context" . ) -}}

  {{- /* Determine if this is a SHA digest reference */ -}}
  {{- $isDigest := hasPrefix "sha256:" $version -}}
  {{- $separator := ternary "@" ":" $isDigest -}}

  {{- if $registry -}}
    {{- printf "%s/%s%s%s" $registry $repository $separator $version -}}
  {{- else -}}
    {{- printf "%s%s%s" $repository $separator $version -}}
  {{- end -}}
{{- end -}}
