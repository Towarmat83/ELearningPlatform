---
title: "Linux Basics: Navigation & Fichiers"
type: interactive
docker_image: ubuntu:22.04
points: 100
order_index: 0
is_published: true
description: Explore le système de fichiers Linux, manipule des fichiers et des répertoires, et découvre les commandes essentielles.
---

## 1. Où suis-je ? (pwd & ls)

La première chose à faire dans un terminal Linux est de savoir **où tu te trouves** et **ce qu'il y a autour de toi**.

- `pwd` (**P**rint **W**orking **D**irectory) affiche le chemin du répertoire actuel.
- `ls` liste les fichiers du répertoire courant.
- `ls -la` affiche **tous** les fichiers (y compris les cachés) avec les permissions et les tailles.

```bash
pwd # Affiche le répertoire courant
ls # Liste les fichiers visibles
ls -la # Liste tous les fichiers avec permissions, tailles et dates
```

## 2. Se déplacer (cd)

`cd` (**C**hange **D**irectory) te permet de naviguer dans l'arborescence.

- `cd /` va à la racine du système.
- `cd /home` va dans le répertoire `/home`.
- `cd ..` remonte d'un niveau.
- `cd ~` ou simplement `cd` revient dans ton répertoire personnel.

```bash
cd / # Va à la racine du système de fichiers
ls # Vois ce qu'il y a à la racine
cd /home # Va dans /home
cd .. # Remonte d'un niveau
cd ~ # Retourne dans ton répertoire personnel
```

## 3. Créer et organiser des fichiers

Voici les commandes essentielles pour **créer**, **lire** et **supprimer** des fichiers :

- `touch nom` crée un fichier vide.
- `mkdir dossier` crée un répertoire.
- `echo "texte" > fichier` écrit du texte dans un fichier.
- `cat fichier` affiche le contenu d'un fichier.

```bash
mkdir mon_dossier # Crée un répertoire nommé mon_dossier
cd mon_dossier # Entre dans ce répertoire
touch notes.txt # Crée un fichier vide nommé notes.txt
echo "Bonjour Linux !" > notes.txt # Écrit du texte dans notes.txt
cat notes.txt # Affiche le contenu de notes.txt
```

## 4. Copier, déplacer et supprimer

- `cp source destination` copie un fichier ou un répertoire.
- `mv source destination` déplace ou renomme un fichier.
- `rm fichier` supprime un fichier.
- `rm -r dossier` supprime un dossier et tout son contenu.

```bash
cp notes.txt notes_backup.txt # Copie notes.txt en notes_backup.txt
mv notes_backup.txt sauvegarde.txt # Renomme le fichier
ls -la # Vérifie les fichiers présents
rm sauvegarde.txt # Supprime sauvegarde.txt
ls # Confirme la suppression
```

## 5. Rechercher et explorer

- `find / -name motif` recherche un fichier par son nom dans tout le système.
- `grep 'motif' fichier` recherche un motif textuel dans un fichier.
- `man commande` affiche le manuel d'une commande (appuie sur `q` pour quitter).
- `history` affiche les commandes récentes.

```bash
grep "Linux" notes.txt # Cherche le mot Linux dans notes.txt
find / -name "*.txt" 2>/dev/null # Trouve tous les fichiers .txt du système
history # Affiche les dernières commandes tapées
echo $PATH # Affiche les répertoires de commandes disponibles
```
