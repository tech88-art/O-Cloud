{{/*
Expand the name of the chart.
*/}}
{{- define "ascend-npu-exporter-plus.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully-qualified app name.
Truncated at 63 characters because some K8s name fields are limited to that.
*/}}
{{- define "ascend-npu-exporter-plus.fullname" -}}
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
{{- define "ascend-npu-exporter-plus.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels — emitted on every object created by this chart.
*/}}
{{- define "ascend-npu-exporter-plus.labels" -}}
helm.sh/chart: {{ include "ascend-npu-exporter-plus.chart" . }}
{{ include "ascend-npu-exporter-plus.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end -}}

{{/*
Selector labels — narrow set used by DaemonSet/Service selectors so
label evolution doesn't break match.
*/}}
{{- define "ascend-npu-exporter-plus.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ascend-npu-exporter-plus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
