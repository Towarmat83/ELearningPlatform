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
  hidden: false
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
                    ├─ GET /internal/enrollments/check?user_id=&course_slug=  → bool
                    ├─ GET /internal/progress/viewed?user_id=&course_slug=     → [lesson_slug]
                    └─ POST /internal/progress/complete                           → mark complete
```

## Contrat API interne (Course → User)

| Méthode | Path | Usage |
|---|---|---|---|
| `GET` | `/internal/enrollments/check?user_id=X&course_slug=Y` | Vérifier si enrolled |
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
- Upload et serve de médias (vidéo, image)
- Appels HTTP vers User Service pour enrollments et progress

## JWT

Clé secrète partagée entre les deux services. Seul User Service produit les tokens, Course Service les valide.

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
