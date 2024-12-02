{{/*
Expand the name of the chart.
*/}}
{{- define "orderly-ape.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "orderly-ape.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "orderly-ape.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "orderly-ape.labels" -}}
helm.sh/chart: {{ include "orderly-ape.chart" . }}
{{ include "orderly-ape.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "orderly-ape.selectorLabels" -}}
app.kubernetes.io/name: {{ include "orderly-ape.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "orderly-ape.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "orderly-ape.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "orderly-ape.hasEnvVar" -}}
{{- $name := (index . 0) -}}
{{- $found := false -}}
{{- with (index . 1) -}}
{{- range $var := .Values.env }}
{{- if eq $var.name $name }}{{ $found = true }}{{ end -}}
{{- end }}
{{- end }}
{{- if $found }}{{ $name }}{{ end -}}
{{- end }}

{{/*
Looks if there's an existing secret and reuse its password. If not it generates
new password and use it.
*/}}
{{- define "orderly-ape.password" -}}
{{- $secret := lookup "v1" "Secret" .Release.Namespace ( include "orderly-ape.fullname" . ) }}
{{- if $secret }}
{{- index $secret "data" "admin-password" }}
{{- else }}
{{- ( randAlphaNum 40 ) | b64enc | quote }}
{{- end }}
{{- end }}
