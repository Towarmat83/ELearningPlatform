# Refactoring GitOps — Content as Code

## Objectif

Migrer la plateforme vers un modèle "content as code" :
- Les cours et leçons sont définis dans des **fichiers** (Markdown + YAML)
- La base de données ne stocke que les **données utilisateurs** (comptes, inscriptions, progression)
- Les instructeurs peuvent connecter un **dépôt git** (GitHub, GitLab, Gitea, self-hosted) pour gérer leurs cours

---

## Ce qui change

### Base de données
- Suppression des tables : `courses`, `labs`, `lessons`, `lab_submissions`, `lab_progress`, `lab_instances`
- Recréation de `enrollments` avec `course_slug TEXT` (au lieu de `course_id UUID`)
- Nouvelle table `lesson_progress(user_id, course_slug, lesson_slug)`
- Nouvelle table `git_repos(id, user_id, url, branch, token_enc, status, error_message, last_synced_at)`

### API (Go)
- Nouveau package `internal/content/` : chargement des cours depuis le filesystem
- Réécriture de `handlers/courses.go` et `handlers/lessons.go` (lecture depuis le store mémoire)
- Nouveau `handlers/repos.go` : CRUD + sync des dépôts git
- Suppression de `handlers/labs.go`, `handlers/submissions.go`, `handlers/instances.go`
- Routes mises à jour dans `handlers/router.go`

### Format de fichier (cours)
```
courses/
  {slug}/
    course.yaml       ← métadonnées du cours
    01-leçon.md       ← leçons (ordre = préfixe numérique)
    02-leçon.md
```

`course.yaml` :
```yaml
title: "Titre du cours"
description: "Description courte"
category: linux
difficulty: beginner
is_published: true
```

Leçon (`01-intro.md`) :
```markdown
---
title: "Introduction"
---

Contenu en **Markdown** libre.
```

### Frontend (SvelteKit)
- Mise à jour de `api.ts` (nouveaux types et endpoints)
- `/courses` et `/courses/[slug]` : affichage depuis le filesystem
- `/courses/[slug]/lessons/[lesson]` : rendu Markdown
- `/dashboard/repos` : gestion des dépôts git

---

## Statut des tâches

### Backend
- [x] Branche `feat/gitops-content` créée
- [x] Migration `010_gitops.sql`
- [x] Package `internal/content/` (types, loader, crypto)
- [x] `internal/config/config.go` mis à jour
- [x] `internal/handlers/state.go` mis à jour
- [x] `handlers/courses.go` réécrit
- [x] `handlers/lessons.go` réécrit
- [x] `handlers/repos.go` créé
- [x] `handlers/router.go` mis à jour
- [x] `main.go` mis à jour
- [x] `labs.go`, `submissions.go`, `instances.go` supprimés

### Contenu exemple
- [x] `courses/linux-intro/course.yaml`
- [x] `courses/linux-intro/01-what-is-linux.md`
- [x] `courses/linux-intro/02-navigation.md`
- [x] `courses/linux-intro/03-files.md`

### Frontend
- [x] `api.ts` mis à jour (types + endpoints repos)
- [x] `/courses` page mise à jour
- [x] `/courses/[slug]` page réécrite
- [x] `/courses/[slug]/lessons/[lesson]` page réécrite
- [x] `/dashboard/repos` page créée

---

## Variables d'environnement ajoutées

| Variable | Défaut | Description |
|----------|--------|-------------|
| `COURSES_DIR` | `./courses` | Répertoire des cours locaux |
| `REPOS_DIR` | `./data/repos` | Répertoire de clonage des dépôts git |
| `REPO_TOKEN_SECRET` | `change-me` | Secret de chiffrement AES-256 des tokens git |

---

## Vérification

```bash
# Lancer la stack
docker compose up --build

# Ou en local
cd api-go && COURSES_DIR=../courses cargo run
cd frontend && npm run dev

# Tester
# 1. GET /api/courses → doit retourner le cours linux-intro
# 2. S'inscrire à un cours → POST /api/courses/linux-intro/enroll
# 3. Voir les leçons → GET /api/courses/linux-intro/lessons
# 4. Marquer une leçon → POST /api/courses/linux-intro/lessons/what-is-linux/complete
# 5. Ajouter un repo git → POST /api/my/repos
# 6. Synchroniser → POST /api/my/repos/{id}/sync
```
