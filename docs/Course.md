Un cours est un objet constitué d'une liste de modules, d'une description, d'un titre, d'un état de publication, d'une catégorie et d'une difficulté.

### Définition

Un cours est défini dans une CRD Kubernetes (`elearning.example.com/v1`, kind `Course`) :

```yaml
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Learn the fundamentals of Kubernetes"
  hidden: false
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "What is Kubernetes"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
    - name: "Architecture Overview"
      type: "video"
      src: "/uploads/architecture.mp4"
    - name: "Kubernetes Basics Quiz"
      type: "quiz"
      passing_score: 80
      max_attempts_per_question: 3
      lock_on_max_attempts: true
      cooldown:
        strategy: "exponential"
        base_seconds: 30
        multiplier: 2.0
        max_seconds: 600
      questions:
        - id: "q1"
          type: "single"
          points: 1
          question: "What is a Pod?"
          answers:
            - id: "a"
              text: "Smallest deployable unit"
              correct: true
            - id: "b"
              text: "A physical node"
              correct: false
```

Module types: `text` (markdown depuis git), `video` / `image` (URL hébergée sur le serveur), `quiz` (questions inline ou YAML depuis git).

- `metadata.name` : slug du cours (utilisé dans les URLs)
- `spec.title` : titre du cours
- `spec.description` : description du cours
- `spec.hidden` : boolean — `true` = caché (non publié), `false` = publié
- `spec.category` : catégorie du cours (ex: kubernetes, linux)
- `spec.difficulty` : niveau de difficulté (beginner, intermediate, advanced)
- `spec.modules[].name` : nom du module
- `spec.modules[].type` : type de module (video, text, image)
- `spec.modules[].src` : URL de la ressource (git pour text, URL serveur pour video/image)
- `spec.modules[].ref` : branche git (uniquement pour type: text)
- `spec.modules[].path` : chemin du fichier dans le dépôt (uniquement pour type: text)
- `spec.modules[].replication` : boolean (optionnel) — si `true`, le serveur télécharge la ressource distante (video/image) et la sert localement via `/uploads/`

### Endpoints

#### GET /api/courses/{slug}/modules

Retourne la liste des modules avec le statut `viewed` par utilisateur :

```json
{
    "modules": [
        {
            "index": 0,
            "slug": "what-is-kubernetes",
            "name": "Qu'est-ce que Kubernetes ?",
            "type": "text",
            "viewed": false
        },
        {
            "index": 1,
            "slug": "core-concepts",
            "name": "Concepts fondamentaux",
            "type": "text",
            "viewed": false
        }
    ]
}
```

#### GET /api/courses/{slug}/modules/{index}

Retourne un module avec son contenu :

```json
{
    "index": 0,
    "slug": "what-is-kubernetes",
    "name": "Qu'est-ce que Kubernetes ?",
    "type": "text",
    "content": "# Markdown content...",
    "viewed": false
}
```

Le champ `viewed` est relatif aux données personnelles stockées en base de données.
Le champ `slug` est la version DNS-compliant du nom du module.
