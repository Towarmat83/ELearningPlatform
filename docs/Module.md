Un module est un élément d'un cours. Il est composé d'un nom, d'une source, d'une référence, d'un type et d'un chemin (définis dans la CRD du cours). Il représente une ressource qui compose un cours : texte (markdown), vidéo ou image.

- **video / image** : l'URL de la ressource hébergée sur le serveur est renvoyée directement
- **text** : le contenu est fetché depuis un dépôt git (si `src`, `ref` et `path` sont renseignés)

### Endpoint

```
GET /api/courses/{slug}/modules/{index}
```

Réponse :

```json
{
    "index": 0,
    "slug": "module-slug",
    "name": "Module Title",
    "type": "text",
    "content": "## Markdown content\n\nHello world",
    "viewed": false
}
```
