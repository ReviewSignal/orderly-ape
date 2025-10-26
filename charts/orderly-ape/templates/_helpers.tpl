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
Create the name of the cert-manager Issuer to use
*/}}
{{- define "orderly-ape.issuerName" -}}
{{- if .Values.issuer.create }}
{{- default (include "orderly-ape.fullname" .) .Values.issuer.name }}
{{- else }}
{{- default "default" .Values.issuer.name }}
{{- end }}
{{- end }}

{{- define "orderly-ape.grafanaURL" -}}
{{- printf "http://%s.%s.svc.cluster.local:%d" (include "grafana.fullname" .Subcharts.grafana) (include "grafana.namespace" .Subcharts.grafana) (int .Values.grafana.service.port) -}}
{{- end }}

{{/*
Grafana environment variables when Grafana is enabled
*/}}
{{- define "orderly-ape.grafanaEnv" -}}
{{- if .Values.grafana.enabled }}
- name: GRAFANA_URL
  value: {{ include "orderly-ape.grafanaURL" . }}
- name: GRAFANA_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ include "orderly-ape.fullname" . }}-grafana-token
      key: admin-token
{{- end }}
{{- end }}

{{/*
InfluxDB address
*/}}
{{- define "orderly-ape.influxdbAddress" -}}
{{- if .Values.influxdb.enabled }}
{{- if .Values.influxdb.ingress.enabled }}
{{- printf "%s://%s" (.Values.influxdb.ingress.tls | ternary "https" "http") .Values.influxdb.ingress.hostname -}}
{{- else }}
{{- printf "http://%s.%s.svc.cluster.local:%d" (include "influxdb.fullname" .Subcharts.influxdb) .Release.Namespace (int .Values.influxdb.service.port) -}}
{{- end }}
{{- end }}
{{- end }}

{{/*
JWT RSA Private Key
*/}}
{{- define "orderly-ape.rsaPrivateKey" -}}
{{- if .Values.jwt.rsaPrivateKey -}}
{{- .Values.jwt.rsaPrivateKey -}}
{{- else -}}
{{- $secret := (lookup "v1" "Secret" .Release.Namespace (include "orderly-ape.fullname" .)) -}}
{{- if (and $secret (hasKey $secret.data "private-key.pem")) -}}
{{- index $secret "data" "private-key.pem" | b64dec -}}
{{- else -}}
{{- genPrivateKey "rsa" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "orderly-ape.victoriaLogsURL" -}}
{{- printf "http://%s.%s.svc.cluster.local:%d" (include "vm.plain.fullname" (dict "helm" .Subcharts.victorialogs "appKey" "server")) .Release.Namespace (int .Values.victorialogs.server.service.servicePort) -}}
{{- end }}

{{/*
Base URL
*/}}
{{- define "orderly-ape.baseURL" -}}
{{- if .Values.baseURL -}}
{{- .Values.baseURL -}}
{{- else -}}
{{- $host := first .Values.ingress.hosts -}}
{{- if $host -}}
http{{ if (or .Values.ingress.tls .Values.issuer.create) }}s{{ end }}://{{ $host.host }}
{{- else -}}
{{- "http://orderly-ape.local" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Dex Issuer
*/}}
{{- define "orderly-ape.dexIssuer" -}}
{{- include "orderly-ape.baseURL" . -}}/dex
{{- end -}}

{{/*
Orderly Ape Admin Email
*/}}
{{- define "orderly-ape.adminEmail" -}}
{{- if .Values.adminEmail -}}
{{- .Values.adminEmail -}}
{{- else -}}
{{- $secret := (lookup "v1" "Secret" .Release.Namespace (printf "%s-admin" (include "orderly-ape.fullname" .))) -}}
{{- if (and $secret (hasKey $secret.data "ADMIN_EMAIL")) -}}
{{- index $secret "data" "ADMIN_EMAIL" | b64dec -}}
{{- else -}}
{{- $base := urlParse (include "orderly-ape.baseURL" .) -}}
{{- printf "admin@%s" $base.host -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Orderly Ape Admin Password
*/}}
{{- define "orderly-ape.adminPassword" -}}
{{- if .Values.adminPassword -}}
{{- .Values.adminPassword -}}
{{- else -}}
{{- $secret := (lookup "v1" "Secret" .Release.Namespace (printf "%s-admin" (include "orderly-ape.fullname" .))) -}}
{{- if (and $secret (hasKey $secret.data "ADMIN_PASSWORD")) -}}
{{- index $secret "data" "ADMIN_PASSWORD" | b64dec -}}
{{- else -}}
{{- randAlphaNum 12 -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Dex Client ID
*/}}
{{- define "orderly-ape.dexClientID" -}}
{{- if .Values.dex.clientID -}}
{{- .Values.dex.ClientID -}}
{{- else -}}
{{- $secret := (lookup "v1" "Secret" .Release.Namespace (printf "%s-oidc-dex" (include "orderly-ape.fullname" .))) -}}
{{- if (and $secret (hasKey $secret.data "OIDC_CLIENT_ID")) -}}
{{- index $secret "data" "OIDC_CLIENT_ID" | b64dec -}}
{{- else -}}
{{- "orderly-ape" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Dex Client Secret
*/}}
{{- define "orderly-ape.dexClientSecret" -}}
{{- if .Values.dex.clientID -}}
{{- .Values.dex.ClientID -}}
{{- else -}}
{{- $secret := (lookup "v1" "Secret" .Release.Namespace (printf "%s-oidc-dex" (include "orderly-ape.fullname" .))) -}}
{{- if (and $secret (hasKey $secret.data "OIDC_CLIENT_SECRET")) -}}
{{- index $secret "data" "OIDC_CLIENT_SECRET" | b64dec -}}
{{- else -}}
{{- printf "%s" (randAlphaNum 32) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
JTW Issuer
*/}}
{{- define "orderly-ape.jwtIssuer" -}}
{{- .Values.jwt.issuer | default (include "orderly-ape.baseURL" .) -}}
{{- end -}}
