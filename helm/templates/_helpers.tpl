{{/*
Expand the chart name.
*/}}
{{- define "webterm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "webterm.fullname" -}}
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
{{- define "webterm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common selector labels shared by every workload.
*/}}
{{- define "webterm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "webterm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels with a workload component.
*/}}
{{- define "webterm.componentSelectorLabels" -}}
{{ include "webterm.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Common labels with a workload component.
*/}}
{{- define "webterm.componentLabels" -}}
helm.sh/chart: {{ include "webterm.chart" .root }}
{{ include "webterm.componentSelectorLabels" . }}
{{- if .root.Chart.AppVersion }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
{{- end }}

{{/*
Generic component resource name helper.
*/}}
{{- define "webterm.componentFullname" -}}
{{- printf "%s-%s" (include "webterm.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "webterm.webserverFullname" -}}
{{- include "webterm.componentFullname" (dict "root" . "component" "webserver") -}}
{{- end }}

{{- define "webterm.webserverTestNodePortFullname" -}}
{{- printf "%s-nodeport" (include "webterm.webserverFullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "webterm.managerFullname" -}}
{{- include "webterm.componentFullname" (dict "root" . "component" "manager") -}}
{{- end }}

{{- define "webterm.terminalsFullname" -}}
{{- include "webterm.componentFullname" (dict "root" . "component" "terminals") -}}
{{- end }}

{{- define "webterm.terminalsHeadlessServiceName" -}}
{{- printf "%s-headless" (include "webterm.terminalsFullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create the name of the service account to use for the manager.
*/}}
{{- define "webterm.managerServiceAccountName" -}}
{{- if .Values.manager.serviceAccount.create }}
{{- default (include "webterm.managerFullname" .) .Values.manager.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.manager.serviceAccount.name }}
{{- end }}
{{- end }}
