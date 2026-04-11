# Labs interactifs — Guide complet

Les labs interactifs permettent de provisionner un container Docker directement dans le navigateur de l'étudiant. L'étudiant obtient un terminal live dans lequel il peut explorer, exécuter des commandes, et chercher des flags — sans rien installer sur sa machine.

---

## Sommaire

1. [Prérequis](#prérequis)
2. [Architecture](#architecture)
3. [Créer un lab interactif (admin)](#créer-un-lab-interactif-admin)
4. [Construire une image Docker pour un lab](#construire-une-image-docker-pour-un-lab)
5. [Exemples d'images](#exemples-dimages)
6. [Utilisation côté étudiant](#utilisation-côté-étudiant)
7. [Tester un lab interactif](#tester-un-lab-interactif)
8. [Limites et sécurité](#limites-et-sécurité)
9. [Dépannage](#dépannage)

---

## Prérequis

### Côté hôte (machine qui fait tourner la plateforme)

| Prérequis | Détail |
|-----------|--------|
| **Docker Engine** | Le daemon Docker doit tourner sur la machine hôte. Testé avec Docker 24+. |
| **Socket Docker exposé à l'API** | Le `docker-compose.yml` monte déjà `/var/run/docker.sock` dans le conteneur API. |
| **Image disponible** | L'image Docker du lab doit être pullable depuis l'hôte (Docker Hub, registry privé, ou image locale déjà présente). |

> Le daemon Docker est utilisé directement via le socket Unix — pas de Docker-in-Docker. Le container du lab tourne au niveau de l'hôte, **pas** à l'intérieur du container API.

### Côté image Docker du lab

L'image doit impérativement contenir **un shell** accessible à la racine :

```
/bin/sh    ← utilisé par défaut (présent dans toutes les images Linux)
```

Si vous voulez bash : assurez-vous que `bash` est installé dans l'image et modifiez éventuellement la configuration si `/bin/sh` pointe vers dash.

---

## Architecture

```
Navigateur (xterm.js)
      │  WebSocket ws://host:8080/ws/courses/.../terminal?token=JWT
      │
   API Rust (Axum)
      │  bollard (Rust Docker SDK)
      │  Unix socket /var/run/docker.sock
      │
   Docker Engine (hôte)
      │
   Container lab (ubuntu:22.04, kali, image custom...)
      └── /bin/sh  ←  stdin/stdout TTY bridgé vers le WebSocket
```

Chaque étudiant a son propre container isolé (réseau coupé, mémoire limitée à 512 MB, 0.5 vCPU). Un seul container par couple `(user, lab)` est autorisé. Le container expire après **30 minutes** (le timer redémarre si l'étudiant relance).

---

## Créer un lab interactif (admin)

### 1. Aller dans l'admin

`http://localhost:3000/admin/courses` → choisir un cours → **+ Add Lab**

### 2. Configurer le lab

| Champ | Valeur recommandée |
|-------|-------------------|
| **Type** | `CTF Challenge` |
| **Title** | Nom du challenge |
| **Description** | Objectif, contexte narratif (Markdown supporté) |
| **Points** | Selon la difficulté |
| **Docker Image** | Nom complet de l'image, ex. `ubuntu:22.04` |
| **Flag** | La valeur exacte que l'étudiant devra soumettre, ex. `FLAG{my_secret}` |
| **Challenge** | Instructions à afficher à l'étudiant (où chercher, quels outils utiliser) |

> Le champ **Docker Image** est optionnel. S'il est vide, le lab fonctionne comme un CTF classique (soumission de flag sans terminal). S'il est rempli, le bouton **▶ Launch Lab** apparaît sur la page du lab côté étudiant.

### 3. Exemple de contenu de challenge (Markdown)

```markdown
Un fichier secret a été laissé quelque part dans ce système.
Trouvez-le et soumettez son contenu.

**Indices :**
- Les fichiers cachés commencent par un point (`.`)
- Regardez dans les répertoires home
- `find` est votre ami
```

---

## Construire une image Docker pour un lab

### Structure de base

```dockerfile
FROM ubuntu:22.04

# Désactiver les prompts apt
ENV DEBIAN_FRONTEND=noninteractive

# Outils disponibles pour l'étudiant
RUN apt-get update && apt-get install -y \
    curl wget net-tools nmap \
    python3 python3-pip \
    vim nano less \
    && rm -rf /var/lib/apt/lists/*

# Créer un utilisateur non-privilégié pour les étudiants
RUN useradd -m -s /bin/bash student
USER student
WORKDIR /home/student

# Placer le flag (à adapter selon votre challenge)
RUN echo "FLAG{find_me_if_you_can}" > /home/student/.hidden_flag

# Garder le container actif (le shell est lancé par l'API via exec)
CMD ["tail", "-f", "/dev/null"]
```

> Le `CMD` sert uniquement à garder le container en vie. Le shell interactif est lancé séparément via `docker exec` — pas depuis le `CMD`.

### Bonne pratique : cacher le flag

Quelques idées selon la difficulté souhaitée :

```bash
# Fichier caché classique
echo "FLAG{level1}" > /home/student/.secretfile

# Dans un binaire (forensics)
python3 -c "import struct; open('/usr/local/bin/challenge', 'wb').write(b'\x7fELF' + b'FLAG{level2}'.encode())"

# Dans une variable d'environnement (si le container est lancé avec)
ENV SECRET_FLAG=FLAG{level3}

# Dans une base SQLite
RUN sqlite3 /home/student/db.sqlite "CREATE TABLE secrets (flag TEXT); INSERT INTO secrets VALUES ('FLAG{level4}');"

# Dans un fichier chiffré (l'étudiant doit trouver la clé ailleurs)
RUN openssl enc -aes-256-cbc -pbkdf2 -k "password123" \
    -in <(echo "FLAG{level5}") -out /home/student/secret.enc
```

### Build et push

```bash
# Build local (pas besoin de registry si la machine hôte build et run)
docker build -t mon-ctf-lab:v1 ./mon-lab/

# Vérifier que le container démarre correctement
docker run -d --name test-lab mon-ctf-lab:v1
docker exec -it test-lab /bin/sh
# → vous devriez avoir un shell

# Nettoyer
docker stop test-lab && docker rm test-lab
```

Si vous utilisez un registry privé :
```bash
docker tag mon-ctf-lab:v1 registry.example.com/labs/mon-ctf-lab:v1
docker push registry.example.com/labs/mon-ctf-lab:v1
```

Dans le champ **Docker Image** de l'admin, saisir : `registry.example.com/labs/mon-ctf-lab:v1`

---

## Exemples d'images

### Image minimale (Alpine)

```dockerfile
FROM alpine:3.19
RUN echo "FLAG{alpine_simple}" > /root/.flag
CMD ["tail", "-f", "/dev/null"]
```

Champ Docker Image : `mon-ctf-alpine:latest`
Taille : ~8 MB, démarrage très rapide.

### Linux forensics (Ubuntu avec outils)

```dockerfile
FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y \
    binutils file strings xxd hexdump \
    gdb ltrace strace \
    python3 sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Challenge : trouver le flag dans un binaire ELF
COPY challenge_binary /usr/local/bin/challenge
RUN chmod +x /usr/local/bin/challenge
CMD ["tail", "-f", "/dev/null"]
```

### Web/réseau (Kali lite)

```dockerfile
FROM kalilinux/kali-rolling
RUN apt-get update && apt-get install -y \
    nmap curl wget netcat-openbsd \
    python3 python3-requests \
    && apt-get clean
CMD ["tail", "-f", "/dev/null"]
```

> Kali est une image lourde (~400 MB). Préférez une image Ubuntu avec uniquement les outils nécessaires pour réduire le temps de démarrage.

---

## Utilisation côté étudiant

1. **Se connecter** et s'inscrire au cours
2. **Ouvrir le lab** → un bandeau "Interactive Environment" apparaît en haut de la page
3. Cliquer sur **▶ Launch Lab** — l'API crée le container et ouvre le terminal (quelques secondes)
4. Le terminal est pleinement interactif : autocomplétion, couleurs ANSI, redimensionnement automatique
5. Trouver le flag, le copier, le coller dans le champ de soumission en bas de page
6. Cliquer sur **Submit** pour valider
7. Cliquer sur **■ Stop** pour arrêter le container (ou le laisser expirer au bout de 30 minutes)

> Si le container est toujours en cours lors d'une prochaine visite de la page, le terminal se reconnecte automatiquement.

---

## Tester un lab interactif

### Checklist de test

#### 1. Vérifier que Docker est accessible depuis l'API

```bash
# Depuis l'hôte
docker ps

# Depuis le container API (si docker compose est lancé)
docker compose exec api sh -c "ls /var/run/docker.sock"
# → /var/run/docker.sock  (doit exister)
```

#### 2. Tester l'image manuellement

```bash
# L'image doit démarrer sans erreur
docker run -d --name lab-test ubuntu:22.04 tail -f /dev/null

# Le shell doit être accessible via exec
docker exec -it lab-test /bin/sh
# → $ prompt attendu

docker stop lab-test && docker rm lab-test
```

#### 3. Tester l'API via curl

```bash
# 1. Se connecter et récupérer le token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@test.com","password":"Test@1234"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# 2. Démarrer une instance (remplacer les UUIDs)
COURSE_ID="<uuid-du-cours>"
LAB_ID="<uuid-du-lab>"

curl -s -X POST "http://localhost:8080/api/courses/$COURSE_ID/labs/$LAB_ID/instance" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Réponse attendue :
# {
#   "instance_id": "...",
#   "status": "running",
#   "expires_at": "2025-..."
# }

# 3. Interroger l'état
curl -s "http://localhost:8080/api/courses/$COURSE_ID/labs/$LAB_ID/instance" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# 4. Arrêter l'instance
curl -s -X DELETE "http://localhost:8080/api/courses/$COURSE_ID/labs/$LAB_ID/instance" \
  -H "Authorization: Bearer $TOKEN"
```

#### 4. Vérifier le container créé

```bash
# Après avoir démarré une instance, lister les containers
docker ps --filter "name=lab-"

# Inspecter les limites appliquées
docker inspect lab-<user-uuid>-<lab-uuid> | python3 -m json.tool | grep -E "Memory|NanoCPUs|NetworkMode"
# Attendu : Memory: 536870912, NanoCPUs: 500000000, NetworkMode: none
```

#### 5. Tester le WebSocket (wscat)

```bash
# Installer wscat si besoin
npm install -g wscat

# Se connecter au terminal WebSocket
wscat -c "ws://localhost:8080/ws/courses/$COURSE_ID/labs/$LAB_ID/terminal?token=$TOKEN"

# Taper des commandes dans le terminal
ls -la
whoami
cat /etc/os-release
```

#### 6. Test de bout en bout dans le navigateur

1. Ouvrir `http://localhost:3000`
2. Se connecter avec un compte étudiant inscrit au cours
3. Ouvrir un lab avec un `docker_image` configuré
4. Cliquer **▶ Launch Lab** → observer le terminal s'ouvrir
5. Taper `ls`, `pwd`, `whoami` → vérifier les réponses
6. Redimensionner la fenêtre → le terminal doit s'adapter
7. Taper `exit` → le terminal se ferme côté container
8. Cliquer **■ Stop** → le container est supprimé

---

## Limites et sécurité

### Limites par container

| Ressource | Limite |
|-----------|--------|
| RAM | 512 MB |
| CPU | 0.5 vCPU |
| PIDs | 50 processus |
| Réseau | **Aucun** (network_mode: none) |
| Durée max | 30 minutes (après quoi l'entrée DB est marquée `stopped`) |

> Le cleanup automatique des containers expirés n'est **pas encore implémenté** sous forme de tâche de fond. Un container expiré sera détecté et nettoyé au prochain `POST` de l'étudiant sur le même lab.

### Ce que le réseau coupé implique

Avec `network_mode: none`, le container **ne peut pas** :
- Accéder à Internet
- Accéder aux autres containers de la plateforme (PostgreSQL, frontend…)
- Faire du port scanning réseau

Si votre lab nécessite un accès réseau local (ex. challenge réseau vers un service interne), vous devrez créer un réseau Docker dédié et l'adapter dans `instances.rs`.

### Isolation supplémentaire recommandée en production

```yaml
# docker-compose.yml — API service
security_opt:
  - no-new-privileges:true
```

Et dans `instances.rs`, ajouter dans `HostConfig` :
```rust
security_opt: Some(vec!["no-new-privileges:true".to_string()]),
cap_drop: Some(vec!["ALL".to_string()]),
cap_add: Some(vec!["CHOWN".to_string(), "SETUID".to_string(), "SETGID".to_string()]),
```

---

## Dépannage

### Le bouton "Launch Lab" n'apparaît pas

- Vérifier que le champ **Docker Image** est bien renseigné dans l'admin pour ce lab
- Vérifier que l'étudiant est bien inscrit au cours

### `Docker not available` à l'appel API

```bash
# Vérifier que le socket est bien monté
docker compose exec api ls -la /var/run/docker.sock

# Vérifier les logs au démarrage de l'API
docker compose logs api | grep -i docker
# → attendu : "Docker daemon connected — interactive labs enabled"
# Si : "Docker not available" → le socket n'est pas accessible
```

### Le container démarre mais le terminal reste blanc

- L'image ne contient peut-être pas `/bin/sh`
- Tester manuellement : `docker run --rm -it <image> /bin/sh`
- Si l'image utilise un shell différent (ex. `/bin/bash` seulement), adapter `instances.rs` ligne `cmd: Some(vec!["/bin/sh".to_string()])` en conséquence

### `Failed to create container: No such image`

L'image n'est pas encore pullée sur l'hôte. Docker ne pull pas automatiquement (pour éviter les délais imprévisibles). Puller manuellement avant le lab :

```bash
docker pull ubuntu:22.04
docker pull kalilinux/kali-rolling
```

Pour puller automatiquement à la création (attention aux délais), vous pouvez ajouter dans `start_instance` avant `create_container` :

```rust
use bollard::image::CreateImageOptions;
use futures_util::TryStreamExt;

let mut pull = docker.create_image(
    Some(CreateImageOptions { from_image: docker_image.as_str(), ..Default::default() }),
    None, None,
);
while let Some(_) = pull.try_next().await.ok().flatten() {}
```

### Le terminal se déconnecte immédiatement

Causes fréquentes :
- L'étudiant n'a plus d'instance en cours (expirée ou stoppée)
- Le token JWT a expiré → se reconnecter

Vérifier l'état de l'instance :
```bash
curl -s "http://localhost:8080/api/courses/$COURSE_ID/labs/$LAB_ID/instance" \
  -H "Authorization: Bearer $TOKEN"
# → { "status": "none" } → plus d'instance active
```

### Nettoyer tous les containers de lab

```bash
# Lister les containers créés par la plateforme
docker ps -a --filter "name=lab-" --format "table {{.Names}}\t{{.Status}}\t{{.CreatedAt}}"

# Arrêter et supprimer tous les containers de lab
docker rm -f $(docker ps -aq --filter "name=lab-") 2>/dev/null || echo "Aucun container à nettoyer"
```
