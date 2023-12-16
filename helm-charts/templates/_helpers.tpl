{{/*
Expand the name of the chart.
*/}}
{{- define "evergreen.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
Expand the name of the chart.
*/}}
{{- define "evergreen.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "evergreen.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "evergreen.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "evergreen.fullname" -}}
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
{{- define "evergreen.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/* Create default labels block */}}
{{- define "evergreen.defaultLabels" }}
  labels:
    app: {{ template "evergreen.name" . }}
    chart: {{ template "evergreen.chart" . }}
    release: {{ .Release.Name }}
    heritage: {{ .Release.Service }}
{{- end -}}



{{/*
Common labels
*/}}
{{- define "evergreen.labels" -}}
    app: {{ template "evergreen.name" . }}
    chart: {{ template "evergreen.chart" . }}
    release: {{ .Release.Name }}
    heritage: {{ .Release.Service }}
{{- end -}}
{{/*
Common pod labels
*/}}
{{- define "evergreen.podlabels" -}}
{{- $labels := .Values.labels | default dict -}}
{{- $common := dict "app.kubernetes.io/part-of" .Release.Name "app.kubernetes.io/instance" .Release.Name "app.kubernetes.io/component" "web-app" "app.kubernetes.io/version" (.Values.image.tag | default ( .Values.image.digest | default "unset:unset" | splitList ":" | last | trunc 7) | toString) -}}
{{- $mergedLabels := merge $labels $common -}}
{{- toYaml $mergedLabels }}
{{- end -}}


{{/*
ServiceAccount name the web-app will use.
*/}}
{{- define "evergreen.serviceAccountName" -}}
{{- printf "%s-%s" .Release.Name .Values.serviceAccount.name -}}
{{- end -}}