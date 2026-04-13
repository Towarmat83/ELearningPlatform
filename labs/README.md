# Labs — LearnLab Content Repository

Ce répertoire contient tous les labs de la plateforme sous forme de fichiers **Markdown**.
Chaque fichier peut être importé directement dans l'interface admin via **Admin → Lab Tools → Markdown → JSON**.

## Structure

```
labs/
  linux/
    01-linux-basics-navigation.md    Interactive — ubuntu:22.04
    02-operation-shadow.md           CTF single-flag — linux-intro:latest
    03-operation-nexus.md            CTF multi-flag  — linux-nexus:latest
    docker/
      linux-intro.Dockerfile         Image Docker pour Opération SHADOW
      linux-nexus.Dockerfile         Image Docker pour Opération NEXUS
```

## Importer un lab dans la plateforme

1. Ouvrir **Admin → Lab Tools → Markdown → JSON**
2. Coller le contenu du fichier `.md`
3. Cliquer **Convert**
4. Cliquer **Save to Library** (optionnel — pour réutilisation)
5. Aller sur **Admin → Courses**, ouvrir un cours, **Add Lab → From Library** (ou JSON tab)

## Format Markdown

### Frontmatter (champs disponibles)

```yaml
---
title: "Titre du lab"              # requis
type: interactive | ctf | form     # requis
docker_image: ubuntu:22.04         # requis pour interactive et ctf avec terminal
points: 100                        # défaut: 100
order_index: 0                     # ordre dans le cours
is_published: false                # défaut: false
description: "..."                 # texte court affiché dans la liste
# CTF seulement :
category: web | crypto | forensics | pwn | reverse | misc
flag: FLAG{secret}                 # CTF single-flag
mode: multi                        # CTF multi-flag (voir section ## Flags)
---
```

### Lab interactif (`type: interactive`)

Chaque `## Titre` devient une **étape**. Le texte libre est la description.
Les blocs ` ```bash ``` ` contiennent les commandes cliquables ; `# commentaire` devient l'explication.

```markdown
## Titre de l'étape

Description de l'étape en **Markdown**.

```bash
commande # Explication affichée à côté du bouton
autre_commande # Autre explication
```
```

### Quiz (`type: form`)

Chaque `## Texte de la question` devient une question.

```markdown
## Quelle commande liste les fichiers ?

- [x] ls          ← réponse correcte (cocher avec [x])
- [ ] dir
- [ ] list

> Explication affichée après soumission (optionnel)

## Question à réponse libre
<!-- type: text -->

**Answer:** la bonne réponse

> Explication (optionnel)
```

### CTF single-flag (`type: ctf`)

Le corps devient la description du challenge. Les sections `## Hints` et `## Resources` sont parsées automatiquement.

```markdown
---
flag: FLAG{secret}
category: web
---

Description du challenge...

## Hints

- Premier indice
- Second indice

## Resources

- [Nom](url)
```

### CTF multi-flag (`type: ctf`, `mode: multi`)

La section `## Flags` contient des sous-sections `### Nom (X pts)` avec `flag: valeur`.

```markdown
---
mode: multi
---

Instructions générales...

## Flags

### Nom du flag (100 pts)
Description de ce flag.
flag: FLAG{valeur_secrete}

### Autre flag (50 pts)
Description.
flag: FLAG{autre_valeur}

## Hints

- Indice général
```

## Construire les images Docker

Les labs CTF nécessitent une image Docker personnalisée :

```bash
# Opération SHADOW
docker build -t linux-intro:latest -f labs/linux/docker/linux-intro.Dockerfile .

# Opération NEXUS
docker build -t linux-nexus:latest -f labs/linux/docker/linux-nexus.Dockerfile .
```

## Ajouter un nouveau lab

1. Créer un fichier `.md` dans le sous-dossier approprié (ex: `labs/web/`)
2. Respecter le format frontmatter + Markdown ci-dessus
3. Si nécessaire, créer le Dockerfile dans `labs/<catégorie>/docker/`
4. Importer via l'interface admin

## Index des labs

| Fichier | Type | Image | Points |
|---------|------|-------|--------|
| `linux/01-linux-basics-navigation.md` | Interactive | `ubuntu:22.04` | 100 |
| `linux/02-operation-shadow.md` | CTF single | `linux-intro:latest` | 150 |
| `linux/03-operation-nexus.md` | CTF multi (4 flags) | `linux-nexus:latest` | 400 |
