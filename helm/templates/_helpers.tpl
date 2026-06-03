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


{{/*
Admin password resolution — priority order:
  1. admin.existingSecret → caller manages the Secret entirely, we do nothing.
  2. admin.password non-empty → use the explicit value from values.yaml.
  3. Secret already exists (upgrade) → reuse ADMIN_PASSWORD from the live Secret
     so we never rotate a previously auto-generated password unintentionally.
  4. First install with no value → generate a cryptographically random 32-char
     alphanumeric password and store it in our platform Secret.
*/}}
{{- define "elearning.adminPassword" -}}
{{- if not .Values.admin.existingSecret -}}
  {{- if .Values.admin.password -}}
    {{- .Values.admin.password -}}
  {{- else -}}
    {{- $secret := lookup "v1" "Secret" .Release.Namespace (printf "%s-secrets" (include "elearning.fullname" .)) -}}
    {{- if and $secret $secret.data (index $secret.data "ADMIN_PASSWORD") -}}
      {{- index $secret.data "ADMIN_PASSWORD" | b64dec -}}
    {{- else -}}
      {{- randAlphaNum 32 -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end }}

{{/*
Database URL
*/}}
{{- define "elearning.databaseUrl" -}}
{{- if .Values.postgresql.enabled -}}
postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}@{{ include "elearning.fullname" . }}-postgresql.{{ .Release.Namespace }}.svc.cluster.local:5432/{{ .Values.postgresql.auth.database }}
{{- else -}}
postgres://{{ .Values.externalDatabase.username }}:{{ .Values.externalDatabase.password }}@{{ .Values.externalDatabase.host }}:{{ .Values.externalDatabase.port }}/{{ .Values.externalDatabase.name }}
{{- end -}}
{{- end }}

{{/*
Name of the secret that holds DATABASE_URL.
When externalDatabase.existingSecret is set, point directly to it.
*/}}
{{- define "elearning.databaseUrlSecretName" -}}
{{- if and (not .Values.postgresql.enabled) .Values.externalDatabase.existingSecret -}}
{{ .Values.externalDatabase.existingSecret }}
{{- else -}}
{{ include "elearning.fullname" . }}-secrets
{{- end -}}
{{- end }}

{{- define "elearning.databaseUrlSecretKey" -}}
{{- if and (not .Values.postgresql.enabled) .Values.externalDatabase.existingSecret -}}
{{ .Values.externalDatabase.existingSecretKey | default "DATABASE_URL" }}
{{- else -}}
DATABASE_URL
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
Default container security context — restrictive, suitable for all services.
Override per-container in values if a service needs specific capabilities.
*/}}
{{- define "elearning.securityContext" -}}
runAsNonRoot: true
runAsUser: 1000
runAsGroup: 1000
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: [ALL]
{{- end }}

