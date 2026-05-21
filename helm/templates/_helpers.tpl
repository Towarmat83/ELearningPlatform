{{/*
Expand the name of the chart.
*/}}
{{- define "elearning.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "elearning.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "elearning.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "elearning.courseService.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: course-service
{{- end }}

{{- define "elearning.userService.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: user-service
{{- end }}

{{- define "elearning.frontend.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: frontend
{{- end }}

{{- define "elearning.prometheus.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: prometheus
{{- end }}

{{- define "elearning.grafana.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: grafana
{{- end }}

{{- define "elearning.ldapService.labels" -}}
{{ include "elearning.labels" . }}
app.kubernetes.io/component: ldap-service
{{- end }}

{{- define "elearning.ldapService.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: ldap-service
{{- end }}

{{/*
Selector labels
*/}}
{{- define "elearning.courseService.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: course-service
{{- end }}

{{- define "elearning.userService.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: user-service
{{- end }}

{{- define "elearning.frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: frontend
{{- end }}

{{- define "elearning.prometheus.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: prometheus
{{- end }}

{{- define "elearning.grafana.selectorLabels" -}}
app.kubernetes.io/name: {{ include "elearning.name" . }}
app.kubernetes.io/component: grafana
{{- end }}

{{/*
Database URL — bitnami chart or external
*/}}
{{- define "elearning.databaseUrl" -}}
{{- if .Values.postgresql.enabled -}}
postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}@{{ include "elearning.fullname" . }}-postgresql:5432/{{ .Values.postgresql.auth.database }}
{{- else -}}
postgres://{{ .Values.externalDatabase.username }}:{{ .Values.externalDatabase.password }}@{{ .Values.externalDatabase.host }}:{{ .Values.externalDatabase.port }}/{{ .Values.externalDatabase.name }}
{{- end -}}
{{- end }}

{{/*
Ingress scheme (http or https)
*/}}
{{- define "elearning.scheme" -}}
{{- if .Values.ingress.tls }}https{{- else }}http{{- end }}
{{- end }}

{{/*
Public base URL of the app (used for CORS, ORIGIN)
*/}}
{{- define "elearning.publicUrl" -}}
{{ include "elearning.scheme" . }}://{{ (first .Values.ingress.hosts).host }}
{{- end }}

{{/*
Prometheus internal service URL
*/}}
{{- define "elearning.prometheusUrl" -}}
http://{{ include "elearning.fullname" . }}-prometheus:{{ .Values.prometheus.service.port }}
{{- end }}
