{{/*
Expand the name of the chart.
*/}}
{{- define "npu-dra-driver.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully-qualified app name.
Truncated at 63 characters because some K8s name fields are limited to that.
*/}}
{{- define "npu-dra-driver.fullname" -}}
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
{{- define "npu-dra-driver.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels — emitted on every object created by this chart.
*/}}
{{- define "npu-dra-driver.labels" -}}
helm.sh/chart: {{ include "npu-dra-driver.chart" . }}
{{ include "npu-dra-driver.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: ocloud-edge
{{- end -}}

{{/*
Selector labels — narrow set used by Deployment/Service selectors so
label evolution doesn't break match.
*/}}
{{- define "npu-dra-driver.selectorLabels" -}}
app.kubernetes.io/name: {{ include "npu-dra-driver.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
ServiceAccount name template.
*/}}
{{- define "npu-dra-driver.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "npu-dra-driver.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
ConfigMap name for the simulator mock NPU JSON.
*/}}
{{- define "npu-dra-driver.mockConfigMapName" -}}
{{- if .Values.mockConfigMap.name -}}
{{- .Values.mockConfigMap.name -}}
{{- else -}}
{{- printf "%s-mock" (include "npu-dra-driver.fullname" .) -}}
{{- end -}}
{{- end -}}
