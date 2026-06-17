{{/*
Expand the name of the chart.
*/}}
{{- define "elearning.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "elearning.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
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
Whether DATABASE_URL is supplied verbatim by a caller-managed Secret.
True only for an external database whose existingSecret exposes a full URL key.
*/}}
{{- define "elearning.databaseUrlFromExternalSecret" -}}
{{- if and (not .Values.postgresql.enabled) .Values.database.auth.existingSecret .Values.database.auth.urlKey -}}
true
{{- end -}}
{{- end }}

{{/*
DATABASE_URL connection string, assembled from the `database` block.
Bundled PostgreSQL → in-cluster Service; otherwise → database.host/port.
*/}}
{{- define "elearning.databaseUrl" -}}
{{- if .Values.postgresql.enabled -}}
postgres://{{ .Values.database.auth.username }}:{{ .Values.database.auth.password }}@{{ include "elearning.fullname" . }}-postgresql.{{ .Release.Namespace }}.svc.cluster.local:5432/{{ .Values.database.name }}
{{- else -}}
postgres://{{ .Values.database.auth.username }}:{{ .Values.database.auth.password }}@{{ .Values.database.host }}:{{ .Values.database.port }}/{{ .Values.database.name }}
{{- end -}}
{{- end }}

{{/*
Name of the Secret that holds DATABASE_URL.
When the external database exposes its own URL Secret, point directly to it;
otherwise the chart renders DATABASE_URL into its own Secret.
*/}}
{{- define "elearning.databaseUrlSecretName" -}}
{{- if eq (include "elearning.databaseUrlFromExternalSecret" .) "true" -}}
{{ .Values.database.auth.existingSecret }}
{{- else -}}
{{ include "elearning.fullname" . }}-secrets
{{- end -}}
{{- end }}

{{- define "elearning.databaseUrlSecretKey" -}}
{{- if eq (include "elearning.databaseUrlFromExternalSecret" .) "true" -}}
{{ .Values.database.auth.urlKey }}
{{- else -}}
DATABASE_URL
{{- end -}}
{{- end }}

{{/*
Name of the Secret that holds JWT_SECRET.
*/}}
{{- define "elearning.jwtSecretName" -}}
{{- if .Values.jwtExistingSecret -}}
{{ .Values.jwtExistingSecret }}
{{- else -}}
{{ include "elearning.fullname" . }}-secrets
{{- end -}}
{{- end }}

{{- define "elearning.jwtSecretKey" -}}
{{- if .Values.jwtExistingSecret -}}
{{ .Values.jwtExistingSecretKey }}
{{- else -}}
JWT_SECRET
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

