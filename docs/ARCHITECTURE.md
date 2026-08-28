# Architecture — E-Learning Platform

## Décision : deux micro-services

Séparation en **User Service** (PostgreSQL) et **Course Service** (stateless).

## Source de vérité des cours : PostgreSQL

La seule source de vérité est la **base de données**. Les cours sont créés et modifiés via l'API d'administration du course-service ; il n'y a ni fichier YAML sur disque, ni ressource Kubernetes, ni cache en mémoire.

Chaque requête lit ce dont elle a besoin :

- le catalogue (`GET /api/courses`) filtre en SQL et ne charge aucune ligne de module ;
- l'affichage d'un cours charge ses modules en une requête indexée sur `(course_slug, position)`.

Aucune réplique ne conserve de contenu en mémoire : une réplique qui vient de démarrer sert exactement le même catalogue qu'une réplique en place depuis une semaine, et un redéploiement ne perd rien.

```jsonc
// POST /api/admin/courses
{
  "slug": "kubernetes-basics",
  "spec": {
    "title": "Kubernetes Basics",
    "description": "Apprenez les bases de Kubernetes",
    "public": true,
    "category": "kubernetes",
    "difficulty": "beginner",
    "modules": [
      { "name": "Introduction", "type": "text",
        "src": "https://github.com/org/courses", "ref": "main", "path": "intro.md" },
      { "name": "Démo vidéo", "type": "video", "src": "/uploads/demo.mp4" }
    ]
  }
}
```

### Schéma

| Table | Rôle |
|---|---|
| `courses` | Métadonnées du cours ; colonnes indexées pour le filtrage catalogue (`category`, `difficulty`, `public`, `title`) et `skills` dénormalisé |
| `course_modules` | Un module = une ligne, ordonnée par `position` ; charges utiles opaques (`questions`, `steps`, `check_params`) en `jsonb` |
| `course_prerequisites` | Conditions d'inscription |
| `course_sessions` | Sessions présentielles, clé `(course_slug, session_id)` — une écriture rejouée écrase la même ligne |
| `paths`, `path_courses`, `path_skills` | Parcours et leurs membres ordonnés |
| `quiz_question_attempts` | Compteur de tentatives et fin de cooldown par question |
| `lab_checks` | Historique des vérifications de labs |

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
          │   (PostgreSQL)    │  │   (PostgreSQL)            │
          │                   │  │                           │
          │  - register       │  │  - catalogue (SQL)        │
          │  - login / JWT    │  │  - liste cours/modules    │
          │  - OAuth          │  │  - git per module         │
          │  - profile        │  │  - upload media           │
          │  - enrollments    │◄─┤  - serve fichiers         │
          │  - lesson_progress│  │  - lab check proxy        │
          │  - platform_sets  │  │  - lab_checks (DB)        │
          │  - users CRUD     │  │                           │
          └────────┬─────────-┘  └───────────┬──────────────┘
                   │                          │
                   │  API REST interne        │  POST /evaluate
                   │                          ▼
                    ├─ GET  /internal/...   ┌──────────────────┐
                    └─ POST /internal/...   │  Checker Service  │
                                            │  (OPA/Rego)       │
                                            │                   │
                                            │  - fetch GitLab   │
                                            │    state via API  │
                                            │  - évalue policy  │
                                            │    Rego           │
                                            └──────────────────┘
```

## Parcours (Paths)

Un parcours est une séquence ordonnée de cours (`kind: course`) ou de compétences (`kind: skill`), stockée dans `paths` + `path_courses` / `path_skills`. Voir `docs/Skills.md` pour le détail.

```jsonc
// POST /api/admin/courses/paths
{
  "slug": "devops-path",
  "spec": {
    "title": "Parcours DevOps",
    "kind": "skill",
    "level": "intermediate",
    "skills": ["linux", "docker", "kubernetes", "ci-cd"]
  }
}
```

## Contrat API interne (Course → User)

| Méthode | Path | Usage |
|---|---|---|
| `GET` | `/internal/enrollments/check?userId=X&courseSlug=Y` | Vérifier si enrolled |
| `POST` | `/internal/enrollments/auto` body: `{userId, courseSlug}` | Auto-enrôler (cours publics) — idempotent |
| `GET` | `/internal/progress/viewed?userId=X&courseSlug=Y` | Récupérer les slugs vus |
| `POST` | `/internal/progress/complete` body: `{userId, courseSlug, lessonSlug}` | Marquer complet |

## Endpoints publics (user-service)

| Méthode | Path | Description |
|---|---|---|
| `GET` | `/api/my/paths` | Parcours de l'utilisateur avec statut par cours/compétence |
| `GET` | `/api/my/skills/{slug}` | Modules d'une compétence avec statut pour l'utilisateur |
| `POST` | `/api/admin/paths/{slug}/enrollments` | Inscrire un utilisateur dans un parcours (admin) |
| `DELETE` | `/api/admin/paths/{slug}/enrollments/{userId}` | Désinscrire (admin) |

## Endpoints publics (course-service)

| Méthode | Path | Description |
|---|---|---|
| `GET` | `/api/paths` | Liste tous les parcours |
| `GET` | `/api/paths/{slug}` | Détail d'un parcours |
| `GET` | `/api/skills/{slug}/modules` | Modules enseignant une compétence |

## Périmètre des services

### User Service (nouveau binaire)

Tout ce qui est DB + auth + relations user↔course.

### Course Service

- Lit les cours et modules depuis PostgreSQL, à la demande
- Résout le contenu des modules :
  - `type: video/image` → sert l'URL
  - `type: text` avec `src` git → clone et lit le fichier
  - `type: quiz` → questions inline ou YAML depuis git, avec score
  - `type: modules` → fetche un fichier YAML d'index depuis git et expand les entrées en place (héritage `src`/`ref` depuis le module parent)
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
| **course-service** | `/etc/course-service/config.yaml` | PORT, CORS_ORIGINS, USER_SERVICE_URL, JWT_SECRET (Secret), DATABASE_URL (Secret) |
| **user-service** | `/etc/user-service/config.yaml` | CORS_ORIGINS, JWT_SECRET (Secret), DATABASE_URL (Secret) |
| **frontend** | `/etc/frontend/config.env` (format `KEY=VALUE`) | PORT, NODE_ENV, ORIGIN, PUBLIC_API_URL, INTERNAL_API_URL, COURSE_API, USER_API |

Le frontend utilise un script d'entrée (`docker-entrypoint.sh`) qui source le fichier `.env` du ConfigMap avant de lancer Node.js.

### Personnalisation

```bash
# Surcharger une valeur via Helm
helm upgrade pupitre helm/ --set courseService.env.GIT_CACHE_TTL=30

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
