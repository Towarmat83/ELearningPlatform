---
title: "Qu'est-ce que Linux ?"
---

## Un système d'exploitation libre

Linux est le noyau (**kernel**) d'un système d'exploitation né en 1991, créé par Linus Torvalds. Il est aujourd'hui au cœur de la quasi-totalité des serveurs, des téléphones Android et de nombreux objets connectés.

Un **système d'exploitation** (OS) est le logiciel qui fait le lien entre le matériel (CPU, RAM, disque) et les programmes que tu utilises. Linux gère :

- la mémoire et les processus
- les systèmes de fichiers
- les périphériques (réseau, stockage, clavier…)
- la sécurité et les permissions

## Les distributions Linux

Le noyau seul ne suffit pas : on l'utilise toujours via une **distribution** (ou *distro*) qui l'empaquette avec un gestionnaire de paquets, des outils système et une interface.

| Distribution | Cas d'usage | Gestionnaire de paquets |
|---|---|---|
| **Ubuntu** | Bureau, débutants | `apt` |
| **Debian** | Serveurs stables | `apt` |
| **Fedora** | Développeurs | `dnf` |
| **Arch Linux** | Utilisateurs avancés | `pacman` |
| **Alpine Linux** | Conteneurs Docker | `apk` |

## Le terminal : ton meilleur allié

Sur Linux, le **terminal** (ou shell) est l'outil central. Contrairement à une interface graphique, il te permet d'automatiser, scripter et contrôler ton système avec précision.

Le shell par défaut sur la plupart des distributions est **Bash** (Bourne Again SHell).

```bash
# Afficher la version du noyau installé
uname -r

# Voir quel shell tu utilises
echo $SHELL

# Afficher l'utilisateur courant
whoami
```

> **Note :** Les lignes qui commencent par `#` sont des commentaires — elles ne s'exécutent pas.

## Pourquoi apprendre Linux ?

- **Omniprésent** : 96 % des serveurs web tournent sous Linux
- **Open source** : tu peux lire, modifier et redistribuer le code
- **Puissant** : automation, scripting, contrôle total du système
- **Gratuit** : aucune licence à payer

Dans les leçons suivantes, tu apprendras à naviguer dans le système de fichiers et à manipuler des fichiers et des permissions.
