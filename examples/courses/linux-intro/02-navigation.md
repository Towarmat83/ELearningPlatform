---
title: "Naviguer dans le système de fichiers"
---

## L'arborescence Linux

Linux organise tous les fichiers dans une seule arborescence qui part de `/` (la **racine**). Il n'y a pas de lettres de lecteurs comme sous Windows — tout est monté sous `/`.

```
/
├── bin/       → commandes essentielles (ls, cp, mv…)
├── etc/       → fichiers de configuration système
├── home/      → répertoires personnels des utilisateurs
│   └── alice/ → répertoire de l'utilisatrice alice
├── tmp/       → fichiers temporaires (vidés au redémarrage)
├── var/       → données variables (logs, bases de données…)
└── usr/       → programmes et bibliothèques installés
```

## Se repérer : pwd et ls

```bash
# Afficher le répertoire courant (Print Working Directory)
pwd

# Lister les fichiers du répertoire courant
ls

# Lister avec détails : permissions, taille, date
ls -l

# Inclure les fichiers cachés (préfixés par un point)
ls -la

# Lister un répertoire spécifique
ls /etc
```

## Se déplacer : cd

```bash
# Aller à la racine
cd /

# Aller dans un répertoire par chemin absolu
cd /home

# Aller dans un sous-répertoire (chemin relatif)
cd mon_dossier

# Remonter d'un niveau
cd ..

# Retourner dans ton répertoire personnel (~)
cd ~
# ou simplement :
cd
```

### Chemins absolus vs relatifs

| Type | Exemple | Description |
|---|---|---|
| **Absolu** | `/home/alice/docs` | Toujours depuis `/` |
| **Relatif** | `docs/rapport.txt` | Depuis le répertoire courant |
| **Spécial `..`** | `../../etc` | Deux niveaux au-dessus |
| **Spécial `~`** | `~/documents` | Depuis ton home |

## Explorer : tree et find

```bash
# Afficher l'arborescence d'un dossier (si tree est installé)
tree /etc -L 2

# Trouver un fichier par son nom (cherche dans tout le système)
find / -name "passwd" 2>/dev/null

# Trouver uniquement dans /etc
find /etc -name "*.conf"

# Trouver les fichiers modifiés dans les dernières 24h
find /var/log -mtime -1
```

> **Astuce :** `2>/dev/null` redirige les erreurs de permission vers `/dev/null` (les ignore) pour un affichage plus propre.

## Raccourcis clavier du terminal

| Raccourci | Action |
|---|---|
| `Tab` | Complétion automatique |
| `↑` / `↓` | Naviguer dans l'historique |
| `Ctrl+C` | Interrompre la commande en cours |
| `Ctrl+L` | Effacer l'écran (équivalent à `clear`) |
| `Ctrl+A` | Aller au début de la ligne |
| `Ctrl+E` | Aller à la fin de la ligne |
