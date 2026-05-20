{{/*
Expand the name of the chart.
*/}}
{{- define "scheduler-plugin.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully-qualified app name.
Truncated at 63 characters because some K8s name fields are limited to that.
*/}}
{{- define "scheduler-plugin.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Chart label — used in helm.sh/chart.
*/}}
{{- define "scheduler-plugin.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels — emitted on every object created by this chart.
*/}}
{{- define "scheduler-plugin.labels" -}}
helm.sh/chart: {{ include "scheduler-plugin.chart" . }}
{{ include "scheduler-plugin.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: ocloud-edge
{{- end -}}

{{/*
Selector labels — narrow set used by Deployment/Service selectors so
label evolution doesn't break match.
*/}}
{{- define "scheduler-plugin.selectorLabels" -}}
app.kubernetes.io/name: {{ include "scheduler-plugin.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
ServiceAccount name template.
*/}}
{{- define "scheduler-plugin.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "scheduler-plugin.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
KubeSchedulerConfiguration ConfigMap name.
*/}}
{{- define "scheduler-plugin.configMapName" -}}
{{- printf "%s-config" (include "scheduler-plugin.fullname" .) -}}
{{- end -}}
