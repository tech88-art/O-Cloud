{{- define "bare-metal-provisioning-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "bare-metal-provisioning-operator.fullname" -}}
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

{{- define "bare-metal-provisioning-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "bare-metal-provisioning-operator.labels" -}}
helm.sh/chart: {{ include "bare-metal-provisioning-operator.chart" . }}
{{ include "bare-metal-provisioning-operator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: ocloud-edge
app.kubernetes.io/component: bare-metal-provisioning-operator
{{- end -}}

{{- define "bare-metal-provisioning-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "bare-metal-provisioning-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "bare-metal-provisioning-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "bare-metal-provisioning-operator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
