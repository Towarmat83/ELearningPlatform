---
title: "Fichiers, permissions et utilisateurs"
---

## Créer et manipuler des fichiers

```bash
# Créer un fichier vide
touch notes.txt

# Créer un répertoire
mkdir mon_projet

# Créer une arborescence complète d'un coup
mkdir -p projet/src/modules

# Écrire du texte dans un fichier (écrase le contenu existant)
echo "Hello Linux" > notes.txt

# Ajouter du texte sans écraser (append)
echo "Deuxième ligne" >> notes.txt

# Afficher le contenu d'un fichier
cat notes.txt

# Afficher avec numéros de ligne
cat -n notes.txt

# Afficher page par page (q pour quitter)
less notes.txt
```

## Copier, déplacer, supprimer

```bash
# Copier un fichier
cp notes.txt notes_backup.txt

# Copier un répertoire entier (récursif)
cp -r mon_projet/ mon_projet_backup/

# Déplacer ou renommer
mv notes.txt documents/notes.txt
mv ancien_nom.txt nouveau_nom.txt

# Supprimer un fichier
rm notes.txt

# Supprimer un répertoire et son contenu
rm -r mon_projet/

# Supprimer sans confirmation (prudence !)
rm -rf dossier_a_supprimer/
```

> ⚠️ `rm -rf` est irréversible — il n'y a pas de corbeille en ligne de commande. Vérifie deux fois avant d'exécuter.

## Comprendre les permissions

Chaque fichier Linux a des permissions définies pour trois entités :

```
-rwxr-xr-- 1 alice devs  4096 jan 15 10:30 script.sh
│└─┬─┘└─┬─┘└─┬─┘
│  │    │    └── autres (others)
│  │    └─────── groupe (group)
│  └──────────── propriétaire (user/owner)
└─────────────── type (- = fichier, d = dossier, l = lien)
```

Chaque triplet `rwx` signifie :
- `r` — **read** : lire le fichier ou lister le dossier
- `w` — **write** : modifier le fichier ou créer des fichiers dans le dossier
- `x` — **execute** : exécuter le fichier ou entrer dans le dossier

## Modifier les permissions : chmod

```bash
# Donner le droit d'exécution au propriétaire
chmod u+x script.sh

# Retirer l'écriture pour le groupe
chmod g-w fichier.txt

# Donner lecture à tous
chmod a+r rapport.txt

# Notation octale (plus rapide pour tout définir d'un coup)
# 7 = rwx | 6 = rw- | 5 = r-x | 4 = r-- | 0 = ---
chmod 755 script.sh   # rwxr-xr-x
chmod 644 config.txt  # rw-r--r--
chmod 600 cle.pem     # rw------- (fichier privé)
```

## Changer le propriétaire : chown

```bash
# Changer le propriétaire d'un fichier
chown bob fichier.txt

# Changer propriétaire ET groupe
chown bob:devs fichier.txt

# Récursif sur un dossier
chown -R www-data:www-data /var/www/
```

> `chown` nécessite généralement les droits `sudo` (superutilisateur).

## Inspecter les fichiers

```bash
# Type d'un fichier
file image.png

# Taille d'un répertoire
du -sh mon_projet/

# Espace disque disponible
df -h

# Compter les lignes d'un fichier
wc -l fichier.txt

# Rechercher un motif dans un fichier
grep "erreur" /var/log/syslog

# Rechercher récursivement dans tous les fichiers .log
grep -r "erreur" /var/log/ --include="*.log"
```
