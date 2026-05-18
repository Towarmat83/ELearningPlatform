# Contributing — Dev Environment Setup

## Prerequisites

Install these tools first:

- [Docker](https://docs.docker.com/engine/install/)
- [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) — `kind` CLI
- [Helm](https://helm.sh/docs/intro/install/) v3 — `helm` CLI
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 22+

Check they are installed:

```bash
docker --version && kind version && helm version --short && kubectl version --client && go version && node --version
```

---

## Quick Start (first time)

### 1. Build all Docker images

```bash
make docker-build
```

This builds 3 images:
- `localhost/elearning-course-service:latest`
- `localhost/elearning-user-service:latest`
- `localhost/elearning-frontend:latest`

If a build fails, check the Dockerfile in each service directory.

### 2. Create the KinD cluster

```bash
make kind-create
```

This creates a local Kubernetes cluster named `elearning` inside Docker.

### 3. Load images into the cluster

```bash
make kind-load
```

### 4. Deploy everything with Helm

```bash
make helm-install
```

This installs all services, PostgreSQL, and the CRD watcher.

### 5. Wait for pods to be ready

```bash
kubectl wait --for=condition=ready pod --all --timeout=180s
```

Check the status:

```bash
kubectl get pods
```

You should see 4 pods running:
- `elearning-course-service-...`
- `elearning-user-service-...`
- `elearning-postgresql-...`
- `elearning-frontend-...`

> **Troubleshooting:** If `elearning-postgresql` stays `Pending`, run:
> ```bash
> kubectl describe pod elearning-postgresql-0
> ```
> If you see `Insufficient ephemeral-storage`, disable Bitnami PostgreSQL and use the standalone one:
> ```bash
> # Edit kind-values.yaml and set: postgresql.enabled: false
> # Then:
> make helm-install
> kubectl apply -f deploy/postgresql.yaml
> ```

### 6. Apply a course

The CRD is already applied by `make helm-install`. Now add a course:

```bash
kubectl apply -f courses/kubernetes-basics/course.yaml
```

### 7. Set up git credentials (for private course repos)

If the course modules reference a private git repo, create a secret:

```bash
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

See `course-service/examples/course-secret.yaml` for the format.

### 8. Expose services locally

```bash
make port-forward
```

### 9. Open the app

Open http://localhost:3000 in your browser.

Default admin login: `admin@elearning.local` / `Admin@1234`

---

## One-command full reset

```bash
make dev
```

This deletes the old cluster, creates a new one, builds images, loads them, runs Helm install, and applies the CRD + course.

---

## Access

| Service       | URL                        | Auth                     |
|---------------|----------------------------|--------------------------|
| Frontend      | http://localhost:3000      | —                        |
| Course API    | http://localhost:18082     | —                        |
| User API      | http://localhost:18081     | —                        |

---

## Makefile commands

| Command                    | What it does                              |
|----------------------------|-------------------------------------------|
| `make kind-create`         | Create KinD cluster (`elearning`)         |
| `make kind-delete`         | Delete KinD cluster                       |
| `make docker-build`        | Build all 3 Docker images                 |
| `make kind-load`           | Load images into KinD nodes               |
| `make helm-install`        | Deploy/upgrade Helm chart                 |
| `make helm-delete`         | Uninstall Helm release                    |
| `make port-forward`        | Expose services on localhost              |
| `make port-forward-stop`   | Stop port-forwards                        |
| `make logs`                | Tail service logs                         |
| `make status`              | Show pods and services                    |
| `make clean`               | Delete Helm release + KinD cluster        |

---

## Rebuilding a single service

```bash
# Build and reload course-service
docker build -t localhost/elearning-course-service:latest course-service/
kind load docker-image localhost/elearning-course-service:latest --name elearning
kubectl rollout restart deploy/elearning-course-service
```

Same pattern for `user-service` and `frontend`.

---

## Adding a new course

Courses are Kubernetes CRDs. Create a YAML file and apply it:

```bash
kubectl apply -f - <<EOF
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: my-new-course
spec:
  title: "My New Course"
  description: "Description here"
  hidden: false
  category: "linux"
  difficulty: "beginner"
  modules:
    - name: "Introduction"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
EOF
```

---

## Troubleshooting

**Port-forwards fail ("address already in use")**:
```bash
make port-forward-stop
make port-forward
```

**Enrollment fails with "Session expired"**: the admin JWT token references a stale user UUID. Log out and log back in.

**Course not found after deployment**: the K8s CRD watcher syncs within a few seconds. Verify:
```bash
kubectl get courses
```

**Blank page / JS errors**: hard refresh the browser (Ctrl+Shift+R) to clear the SvelteKit cache.

**PostgreSQL not starting**: check pod events:
```bash
kubectl describe pod -l app=postgresql
```

**User service CrashLoopBackOff**: check logs:
```bash
kubectl logs -l app.kubernetes.io/name=elearning --tail=20
```
If it says "connection refused" to PostgreSQL, the database isn't ready yet.

**Git content not loading (500 error)**: the git secret `course-repo-secret` may be missing or the token is invalid. Recreate it:
```bash
kubectl delete secret course-repo-secret
kubectl create secret generic course-repo-secret --from-file=git-credentials.yaml=./git-credentials.yaml
```

**ConfigMaps**: after modifying a ConfigMap, restart the pod to pick up the changes:
```bash
kubectl rollout restart deploy/elearning-course-service
kubectl rollout restart deploy/elearning-user-service
kubectl rollout restart deploy/elearning-frontend
```

The Go services read the mounted YAML at startup. The frontend entrypoint sources the `.env` file on each container start. All three also respect env var overrides — see `docs/ARCHITECTURE.md` (section "Configuration des services").
