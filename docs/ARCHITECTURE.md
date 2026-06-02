# Architecture — E-Learning Platform

## Décision : deux micro-services

Séparation en **User Service** (PostgreSQL) et **Course Service** (stateless).

## Source de vérité des cours : CRD Kubernetes

Les cours ne sont plus définis dans des fichiers YAML sur disque ni via l'API backend. La seule source de vérité est le **CRD Kubernetes** `elearning.example.com/v1` (kind `Course`).

Le backend **watche** la K8s API via client-go pour maintenir son store en mémoire.

```yaml
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Apprenez les bases de Kubernetes"
  public: true
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "Introduction"
      type: "text"
      src: "https://github.com/org/courses"
      ref: "main"
      path: "intro.md"
    - name: "Démo vidéo"
      type: "video"
      src: "/uploads/demo.mp4"
```

## Architecture

```
                    Client (Frontend)
                     │              │
               /api/auth/*     /api/courses/*
               /api/admin/*    /api/admin/*
               /api/my/*
                     │              │
                     ▼              ▼
          ┌──────────────────┐  ┌──────────────────────────┐
          │   User Service    │  │   Course Service          │
          │   (PostgreSQL)    │  │   (stateless)             │
          │                   │  │                           │
          │  - register       │  │  - K8s CRD watcher        │
          │  - login / JWT    │  │  - liste cours/modules    │
          │  - OAuth          │  │  - git per module         │
          │  - profile        │  │  - upload media           │
          │  - enrollments    │◄─┤  - serve fichiers         │
          │  - lesson_progress│  │                           │
          │  - platform_sets  │  │  JWT: valide seul         │
          │  - users CRUD     │  │                           │
          └────────┬─────────-┘  └──────────────────────────┘
                   │
                   │  API REST interne
                   │
                    ├─ GET  /internal/enrollments/check?user_id=&course_slug=  → bool
                    ├─ POST /internal/enrollments/auto  {user_id, course_slug}  → auto-enroll (public courses)
                    ├─ GET  /internal/progress/viewed?user_id=&course_slug=     → [lesson_slug]
                    └─ POST /internal/progress/complete                          → mark complete
```

## Contrat API interne (Course → User)

| Méthode | Path | Usage |
|---|---|---|
| `GET` | `/internal/enrollments/check?user_id=X&course_slug=Y` | Vérifier si enrolled |
| `POST` | `/internal/enrollments/auto` body: `{user_id, course_slug}` | Auto-enrôler (cours publics) — idempotent |
| `GET` | `/internal/progress/viewed?user_id=X&course_slug=Y` | Récupérer les slugs vus |
| `POST` | `/internal/progress/complete` body: `{user_id, course_slug, lesson_slug}` | Marquer complet |

## Périmètre des services

### User Service (nouveau binaire)

Tout ce qui est DB + auth + relations user↔course.

### Course Service

- Watche les CRD `Course` depuis l'API Kubernetes
- Résout le contenu des modules :
  - `type: video/image` → sert l'URL
  - `type: text` avec `src` git → clone et lit le fichier
  - `type: quiz` → questions inline ou YAML depuis git, avec score
  - `type: modules` → fetche un fichier YAML d'index depuis git et expand les entrées en place (héritage `src`/`ref` depuis le parent CRD)
- Upload et serve de médias (vidéo, image)
- Appels HTTP vers User Service pour enrollments et progress

## JWT

Clé secrète partagée entre les deux services. Seul User Service produit les tokens, Course Service les valide.

## Configuration des services

Chaque service charge sa configuration depuis une **ConfigMap montée en fichier** dans le conteneur. Les variables d'environnement explicites dans le déploiement coexistent et **surchargent** les valeurs du fichier.

### Mécanisme de chargement

1. Le service Go lit son fichier YAML (`config.yaml`) monté via `CONFIG_PATH`
2. Les variables d'environnement définies dans le déploiement écrasent les valeurs YAML
3. Les secrets sensibles (JWT, DB) restent dans des K8s Secrets

| Service | Fichier ConfigMap monté | Env vars en override |
|---|---|---|
| **course-service** | `/etc/course-service/config.yaml` | PORT, CORS_ORIGINS, USER_SERVICE_URL, JWT_SECRET (Secret) |
| **user-service** | `/etc/user-service/config.yaml` | CORS_ORIGINS, JWT_SECRET (Secret), DATABASE_URL (Secret) |
| **frontend** | `/etc/frontend/config.env` (format `KEY=VALUE`) | PORT, NODE_ENV, ORIGIN, PUBLIC_API_URL, INTERNAL_API_URL, COURSE_API, USER_API |

Le frontend utilise un script d'entrée (`docker-entrypoint.sh`) qui source le fichier `.env` du ConfigMap avant de lancer Node.js.

### Personnalisation

```bash
# Surcharger une valeur via Helm
helm upgrade elearning helm/ --set courseService.env.GIT_CACHE_TTL=30

# Ou modifier les valeurs.yaml puis helm upgrade
```

## Secret git — course-repo-secret

Pour les modules de cours qui référencent un **dépôt git privé**, le course-service a besoin d'un token d'accès. Ce token est fourni via un Secret K8s monté dans le pod :

```
course-repo-secret (type: Opaque)
  └── git-credentials.yaml
        └── credentials:
              - url: "github.com/org/*"        # glob pattern
                token: "github_pat_xxx..."
```

Le fichier suit le format YAML avec une liste d'entrées **url → token**. La première entrée qui correspond (via `path.Match`) est utilisée.

**Création :**
```bash
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

**Chemin de montage dans le pod :** `/etc/git-credentials/git-credentials.yaml`

La variable d'environnement `GIT_CREDENTIALS_PATH` (ConfigMap ou valeur par défaut) indique l'emplacement. Si le secret n'existe pas, le pod démarre mais le clone de repos privés échouera avec une erreur 403.

Voir `infra/examples/course-service/course-secret.yaml` pour un exemple complet.

## CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: courses.elearning.example.com
spec:
  group: elearning.example.com
  names:
    kind: Course
    plural: courses
    singular: course
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
```
