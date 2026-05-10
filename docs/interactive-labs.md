# Labs interactifs — Historique (obsolète)

> **⚠️ Ce document décrit l'ancien système de labs interactifs de l'API Rust (monolithe).**
> Cette fonctionnalité n'existe plus dans l'architecture actuelle (Go, micro-services).
> Les routes `/api/courses/{slug}/labs` existent toujours mais retournent simplement
> les modules d'un cours formatés en labs — sans terminal Docker interactif.

## Ancienne architecture (pour référence)

```
Navigateur (xterm.js)
      │  WebSocket ws://localhost:8080/ws/courses/.../terminal?token=JWT
      │
   API Rust (Axum) — bollard Docker SDK
      │
   Docker Engine (hôte) → Container lab (ubuntu:22.04, etc.)
```

Chaque étudiant avait son propre container isolé (réseau coupé, 512 MB RAM, 0.5 vCPU).
Un seul container par couple `(user, lab)` avec expiration après 30 minutes.

## Si vous cherchez à ajouter des labs interactifs

L'architecture actuelle (Go, course-service) ne supporte pas le provisionnement
de containers Docker. Pour ajouter cette fonctionnalité, il faudrait :

1. Ajouter un package Docker client dans `course-service` (ou un nouveau service dédié)
2. Créer des endpoints WebSocket pour les terminaux interactifs
3. Gérer le cycle de vie des containers (création, heartbeat, nettoyage)

Voir l'ancien code Rust dans l'historique git pour référence d'implémentation.
