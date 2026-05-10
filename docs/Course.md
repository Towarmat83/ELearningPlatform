Un cours est un objet constitué d'une liste de modules, d'une description, d'un titre, d'un état de publication, d'une catégorie et d'une difficulté.

### Définition

Un cours est défini dans une CRD sous le format suivant :

```yaml
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Learn the fundamentals"
  hidden: false
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "Introduction"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
    - name: "Architecture"
      type: "video"
      src: "/uploads/architecture.mp4"
```

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
