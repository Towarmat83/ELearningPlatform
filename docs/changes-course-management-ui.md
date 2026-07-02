# Course & Pattern management UI — change log

Branch: `chore/remove-grafana-prometheus`

---

## Overview

This batch of changes adds a full admin UI for managing **Course CRDs** and **MarkdownPattern CRDs** directly from the browser, without needing `kubectl apply`. It also fixes several bugs discovered during development and testing.

---

## New features

### Course creation / editing from the admin UI

**`frontend/src/routes/admin/courses/+page.svelte`**

Replaced the raw YAML modal with a structured form:

- **+ New course** button (top-right of the Courses page) shows an inline form with fields: Slug (required, validated `[a-z0-9-]+`), Title, Description, Category, Difficulty (select), Public toggle.
- **Edit** button on each course row pre-populates the form from the live CRD spec.
- **Delete** button removes the CRD from the cluster after confirmation.
- **⎈ YAML** button opens a read-only modal showing the full `kubectl apply`-ready YAML.
- Form is disabled / button greyed out when required fields are missing.

### Module management from the admin UI

Same page — **Modules** tab inside each course accordion:

- **+ Add module** button shows an inline form with fields: Name (required), Type (text / video / image / quiz / modules-index), Git repo URL, Branch / ref, Path in repo, Hidden toggle. Quiz ref field appears only when type = `quiz`.
- **Edit** button on each module row pre-populates the form.
- **Delete** button removes the module from the spec after confirmation.
- All operations read the current CRD spec via `getCourseCRD`, mutate the `modules` array in-place, then call `updateCourseCRD`.

### MarkdownPattern admin page

**`frontend/src/routes/admin/patterns/+page.svelte`** (new file)

Full CRUD for `MarkdownPattern` CRDs:

- Inline form: Name, Label, Scope (`global` or a course slug), HTML template (`{{content}}`), CSS, JS.
- Name change auto-updates `md-pat-*` class names in HTML / CSS fields.
- Live preview panel renders the pattern in real time using the Markdown renderer.
- **⎈ YAML** button generates a `kubectl apply`-ready YAML snippet.
- **Edit** and **Delete** per pattern.

**`frontend/src/routes/admin/+layout.svelte`** — added *Patterns* nav link.

### MarkdownPattern CRD definition

**`helm/crds/markdownpattern-crd.yaml`** (new file) — `MarkdownPattern` CRD schema for the cluster.

### Pattern rendering in Markdown

**`frontend/src/lib/markdown.ts`** — added two `marked` extensions:

- **Block**: `|||patternName\n…content…\n|||`
- **Inline**: `|||patternName text|||`

Both replace `{{content}}` in the pattern HTML with the rendered markdown.

**`frontend/src/lib/Markdown.svelte`** — now accepts `courseSlug` and `overridePatterns` props. Loads patterns from the cluster on mount, injects pattern CSS/JS into `<head>` after each render. `overridePatterns` enables live preview without saving.

**`frontend/src/lib/patternStore.ts`** (new file) — Svelte store backed by `patternsApi.list()`, shared across all `Markdown` components in a page load.

---

## Backend — course-service

### Course CRD handlers

**`course-service/internal/handlers/courses_crd.go`** (new file)

- `POST /api/admin/courses` — `CreateCourseCRD`: creates a `Course` CRD in Kubernetes.
- `GET /api/admin/courses/{slug}/crd` — `GetCourseCRD`: returns the raw spec of an existing CRD.
- `PUT /api/admin/courses/{slug}/crd` — `UpdateCourseCRD`: replaces the spec of an existing CRD.
- `DELETE /api/admin/courses/{slug}/crd` — `DeleteCourseCRD`: deletes the CRD from the cluster.

### Pattern CRD handlers

**`course-service/internal/handlers/patterns.go`** (new file)

- `k8sDynamic(kubeconfig)` — shared helper that builds a dynamic K8s client (in-cluster or via kubeconfig).
- `GET /api/patterns` — `ListPatterns`: serves patterns from the in-memory `PatternStore`.
- `POST /api/admin/patterns` — `CreatePatternCRD`
- `PUT /api/admin/patterns/{name}` — `UpdatePatternCRD`
- `DELETE /api/admin/patterns/{name}` — `DeletePatternCRD`

### PatternStore + PatternWatcher

**`course-service/internal/content/patterns.go`** (new file)

- `PatternStore` — thread-safe in-memory cache of `MarkdownPattern` CRs, keyed by `name/scope`.
- `PatternWatcher` — watches `markdownpatterns.elearning.pupitre.io` CRDs via the K8s watch API, syncs on start and on every ADD / MODIFIED / DELETED event.

### Router

**`course-service/internal/handlers/router.go`**

Added routes:

```
GET  /api/patterns
POST /api/admin/patterns
PUT  /api/admin/patterns/{name}
DELETE /api/admin/patterns/{name}
POST /api/admin/courses
GET  /api/admin/courses/{slug}/crd
PUT  /api/admin/courses/{slug}/crd
DELETE /api/admin/courses/{slug}/crd
```

### State

**`course-service/internal/handlers/state.go`** — added `Patterns *content.PatternStore` field.

**`course-service/main.go`** — initialises `PatternStore` and starts `PatternWatcher` alongside the existing `K8sWatcher`.

### Admin bypass for private courses

**`course-service/internal/handlers/courses.go`** — `GetCourse`: admins can now view any private course without being enrolled. Previously, the enrollment check had no admin bypass, so even admins received 404 on private courses they hadn't joined.

---

## Frontend — API client

**`frontend/src/lib/api.ts`**

Added to `adminApi`:

```typescript
createCourse(body, token)       // POST /admin/courses
getCourseCRD(slug, token)       // GET  /admin/courses/{slug}/crd
updateCourseCRD(slug, body, token) // PUT  /admin/courses/{slug}/crd
deleteCourseCRD(slug, token)    // DELETE /admin/courses/{slug}/crd
```

Added new `patternsApi`:

```typescript
patternsApi.list(scope?)
patternsApi.create(pattern, token)
patternsApi.update(name, pattern, token)
patternsApi.delete(name, token)
```

Added `Pattern` interface.

---

## Proxy routing

**`frontend/src/hooks.server.ts`**

- Routes `/api/admin/courses*` and `/api/admin/patterns*` and `/api/patterns*` to the **course-service**.
- Excludes `/api/admin/courses/{slug}/enrollments*` from the course-service rule so those continue to reach the **user-service**.

Previously, all `/api/admin/courses/{slug}/enrollments` calls were silently routed to the course-service (which has no enrollment routes), returning errors.

---

## Helm / Kubernetes

**`helm/templates/course-service/rbac.yaml`** — added `markdownpatterns` to the list of CRD resources the course-service `ClusterRole` can `get / list / watch / create / update / patch / delete`.

**`helm/templates/user-service/deployment.yaml`** — `automountServiceAccountToken: true` (was `false`).

---

## Bug fixes

| # | File | Description |
|---|------|-------------|
| 1 | `course-service/Dockerfile` | `ENV USER_SERVICE_URL` was baked into the image as `http://user-service:8081`. Kubernetes service DNS is `elearning-user-service`, so `isEnrolled()` always failed silently, returning `false` for every user. Fixed by setting the default to `""` so the value from the configmap (`http://elearning-user-service.default.svc.cluster.local:8081`) is used. |
| 2 | `frontend/Dockerfile` | Runtime stage only copied `/app/build` and `package.json` — no `node_modules`. The `yaml` package (used for YAML serialisation in SSR) was not available at runtime, causing HTTP 500 on every visit to `/admin/courses`. Fixed by adding `COPY package-lock.json` + `RUN npm ci --omit=dev --ignore-scripts` in the runtime stage. |
| 3 | `frontend/src/hooks.server.ts` | Enrollment routes (`/api/admin/courses/{slug}/enrollments`) matched the course-service rule before the user-service rule, causing "Failed to load enrollments" errors. Fixed with a negative lookahead in the course-service routing condition. |
| 4 | `frontend/src/routes/admin/courses/+page.svelte` | `cancelModuleDraft()` set `moduleDraftCourseId = null` before `loadModules(moduleDraftCourseId)` used it, resulting in a `GET /api/courses/null/modules` → "Course not found" error after every successful module save. Fixed by capturing the ID in a local variable before calling `cancelModuleDraft`. |
| 5 | `Makefile` | `IMAGE_COURSE/USER/FRONTEND` variables pointed to `ghcr.io/towarmat83/…:latest`, but the running deployments use `localhost/…:local` (set by `kind-values.yaml`). `make rebuild-*` was loading the wrong image tags, so pods never picked up new builds. Fixed by changing the image variables to `localhost/…:local`. |
| 6 | `frontend/package.json` | `yaml` was a transitive dependency only. Added as an explicit `dependency` so it is reliably available across clean installs. |
