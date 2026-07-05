Un cours est un objet constitué d'une liste de modules, d'une description, d'un titre, d'un état de publication, d'une catégorie et d'une difficulté.

### Définition

Un cours est défini dans une CRD Kubernetes (`elearning.pupitre.io/v1`, kind `Course`) :

```yaml
apiVersion: elearning.pupitre.io/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Learn the fundamentals of Kubernetes"
  public: true
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
      passingScore: 80
      maxAttemptsPerQuestion: 3
      lockOnMaxAttempts: true
      cooldown:
        strategy: "exponential"
        baseSeconds: 30
        multiplier: 2.0
        maxSeconds: 600
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

Module types: `text` (markdown depuis git), `video` / `image` (URL hébergée sur le serveur), `quiz` (questions inline ou YAML depuis git), `modules` (index file — expansion en place de plusieurs modules depuis git).

- `metadata.name` : slug du cours (utilisé dans les URLs)
- `spec.title` : titre du cours
- `spec.description` : description du cours
- `spec.public` : boolean — `true` = visible dans le catalogue et auto-enrôlement à la première visite d'un module ; `false` = cours privé, accessible uniquement aux utilisateurs déjà enrôlés (non visible dans le catalogue public)
- `spec.category` : catégorie du cours (ex: kubernetes, linux)
- `spec.difficulty` : niveau de difficulté (beginner, intermediate, advanced)
- `spec.modules[].name` : nom du module
- `spec.modules[].type` : type de module (`text`, `video`, `image`, `quiz`, `modules`)
- `spec.modules[].src` : URL du dépôt git ou de la ressource média
- `spec.modules[].ref` : branche git
- `spec.modules[].path` : chemin du fichier dans le dépôt
- `spec.modules[].replication` : boolean (optionnel) — si `true`, le serveur télécharge la ressource distante (video/image) et la sert localement via `/uploads/`

#### Type `modules` — inclusion par index file

Pour les cours à nombreux modules hébergés dans git, le type `modules` évite de déclarer chaque module individuellement dans la CRD. Une seule entrée pointe vers un fichier YAML d'index dans git ; le course-service l'expand en place à chaque requête.

```yaml
modules:
  - name: "Linux Introduction"
    type: modules
    src: "https://github.com/org/repo"
    ref: "main"
    path: "courses/linux-intro/index.yaml"
```

Le fichier `index.yaml` liste les modules dans l'ordre d'affichage. Les champs `src` et `ref` sont hérités de l'entrée CRD si absents. Voir `docs/Module.md` pour le format complet de l'index.

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
