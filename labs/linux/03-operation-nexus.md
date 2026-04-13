---
title: "Opération NEXUS"
type: ctf
mode: multi
docker_image: linux-nexus:latest
points: 400
order_index: 2
is_published: true
description: Reconstituez le code d'accès NEXUS en localisant les 4 fragments cachés dans le système.
---

Bienvenue, agent. Votre mission : reconstituer le code d'accès NEXUS en récupérant **4 fragments** dissimulés dans le système.

Le flag final sera de la forme : `FLAG{xxxx_xxxx_xxxx_XXXXX}`

Utilisez uniquement les outils disponibles : `grep`, `cut`, `find`, `tar`, `chmod`, `cat`.

## Flags

### Fragment 1 — Logs système (100 pts)

Une alerte critique a été enregistrée dans les journaux du service `nexus-core`.
Cherchez dans `/var/log/nexus/system.log` les entrées de niveau `ALERT`.

Le fragment se trouve dans le champ `msg=` de la ligne d'alerte.

flag: 4a2f

### Fragment 2 — Base de données agents (100 pts)

Un agent spécial se cache dans la base CSV `/opt/nexus/data/agents.csv`.
Le fichier contient les colonnes : `ID, NAME, STATUS, CODE, REGION`.

Trouvez l'agent `NEXUS_AGENT` et extrayez son `CODE`.

flag: b8c3

### Fragment 3 — Fichier classifié (100 pts)

Un fichier existe à `/opt/nexus/data/classified.txt` mais ses permissions sont verrouillées (`000`).
Vous êtes propriétaire du fichier — modifiez les permissions pour le lire.

flag: 9d1e

### Fragment 4 — Archive dissimulée (100 pts)

Une archive compressée est cachée quelque part sous `/opt/nexus`.
Utilisez `find` pour la localiser (cherchez les fichiers `.tar.gz`, y compris cachés).
Extrayez l'archive dans `/tmp` et lisez le fichier qu'elle contient.

flag: HUNT3R

## Hints

- `grep "ALERT" /var/log/nexus/system.log` — filtre par niveau de log
- `grep "NEXUS_AGENT" /opt/nexus/data/agents.csv | cut -d',' -f4` — extrait le code de l'agent
- `chmod +r /opt/nexus/data/classified.txt` — rend le fichier lisible
- `find /opt/nexus -name "*.tar.gz"` — cherche les archives (ajoutez `-name ".*"` pour les fichiers cachés)
- `tar -xzf <archive> -C /tmp && cat /tmp/HUNT3R.txt` — extraction et lecture

---

> **Build** : `docker build -t linux-nexus:latest -f labs/linux/docker/linux-nexus.Dockerfile .`
