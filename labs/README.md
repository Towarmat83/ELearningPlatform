# Labs — Legacy Content (obsolete)

> **⚠️ Ce répertoire contient d'anciens labs de l'API Rust (monolithe).**
> L'architecture actuelle (Go, micro-services) ne supporte plus les labs
> interactifs avec containers Docker. Les fichiers sont conservés à titre
> de référence uniquement.

## Ancien format

Les labs étaient importés via **Admin → Lab Tools → Markdown → JSON** dans
l'ancienne interface Rust. Ce système n'existe plus.

## Nouveau système

Les cours sont désormais définis comme **CRD Kubernetes** (`elearning.example.com/v1`).
Voir `examples/courses/` pour des exemples de définitions de cours.

```bash
# Appliquer un cours CRD
kubectl apply -f examples/courses/kubernetes-basics/course.yaml
```
