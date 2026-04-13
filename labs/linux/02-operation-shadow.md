---
title: "Opération SHADOW"
type: ctf
category: forensics
flag: FLAG{linux_sh4d0w_hunt3r_2024}
docker_image: linux-intro:latest
points: 150
order_index: 1
is_published: true
description: Une taupe a laissé des traces sur ce système. Retrouvez le code secret caché dans les profondeurs du filesystem.
---

Une taupe a laissé des traces sur ce système. Votre mission :
retrouver le code secret qu'elle a caché dans les profondeurs du filesystem.

Le chemin est balisé en **3 niveaux** — chaque niveau vous rapproche du flag.

Commencez par lire le fichier `README.md` qui se trouve dans votre répertoire courant.

## Hints

- Les répertoires cachés commencent par `.` — utilisez `ls -la` pour les voir
- `cat /opt/mission/data/niveau1.txt` révèle le premier indice
- Les données encodées en base64 se décodent avec `base64 -d fichier`
- Le flag est dans `/var/cache/` — cherchez les fichiers cachés

---

> **Build** : `docker build -t linux-intro:latest labs/linux/docker/linux-intro.Dockerfile`
