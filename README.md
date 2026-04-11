# LearnLab — eLearning Platform

Stack: **Rust (Axum) API** + **SvelteKit** frontend + **PostgreSQL** + **Prometheus/Grafana**

## Configuration — Variables d'environnement

Les fichiers `.env` ne sont **jamais commités**. Copiez les exemples et adaptez les valeurs.

### `.env` — racine (Docker Compose)

```bash
cp .env.example .env
```

| Variable | Description | Exemple / Défaut |
|----------|-------------|-----------------|
| `DB_PASSWORD` | Mot de passe PostgreSQL | `elearning_secret` |
| `JWT_SECRET` | Clé secrète JWT (min. 32 chars) | à changer en prod |
| `DATABASE_URL` | URL complète de connexion PostgreSQL | `postgres://elearning:<DB_PASSWORD>@localhost:5432/elearning` |
| `FRONTEND_URL` | URL publique du frontend (CORS) | `http://localhost:3000` |
| `GRAFANA_PASSWORD` | Mot de passe admin Grafana | `admin` |

### `api/.env` — développement local uniquement

```bash
cp api/.env.example api/.env
```

| Variable | Description | Exemple / Défaut |
|----------|-------------|-----------------|
| `DATABASE_URL` | URL PostgreSQL (port 5433 pour Docker local) | `postgres://elearning:elearning_secret@localhost:5433/elearning` |
| `JWT_SECRET` | Clé secrète JWT (min. 32 chars) | à changer en prod |
| `JWT_EXPIRY_HOURS` | Durée de validité des tokens | `24` |
| `PORT` | Port d'écoute de l'API | `8080` |
| `RUST_LOG` | Niveau de logs | `info,elearning_api=debug` |
| `CORS_ORIGINS` | Origines autorisées (séparées par virgule) | `http://localhost:3000,http://localhost:5173` |

> En production, ne jamais laisser `JWT_SECRET` avec la valeur par défaut. Générez une clé aléatoire : `openssl rand -hex 32`

---

## Authentification SSO (OAuth2)

LearnLab supporte l'authentification via **GitLab** et **GitHub** en parallèle du login local (email + mot de passe). Les deux modes coexistent et peuvent être activés ou désactivés indépendamment depuis le panneau d'administration.

---

### Comment fonctionne le SSO

Le flux utilisé est le standard **OAuth2 Authorization Code** :

```
Utilisateur          Frontend              API (Rust)           Provider (GitLab/GitHub)
    │                    │                     │                         │
    │  Clic "Continue    │                     │                         │
    │  with GitLab"      │                     │                         │
    │───────────────────▶│                     │                         │
    │                    │  GET /api/auth/      │                         │
    │                    │  oauth/gitlab/       │                         │
    │                    │  authorize           │                         │
    │                    │────────────────────▶│                         │
    │                    │  { url, state }      │                         │
    │                    │◀────────────────────│                         │
    │                    │                     │                         │
    │  Redirect vers     │                     │                         │
    │  provider          │                     │                         │
    │◀───────────────────│                     │                         │
    │                    │                     │                         │
    │  Connexion +        │                    │                         │
    │  consentement      │                     │                         │
    │─────────────────────────────────────────────────────────────────▶│
    │                    │                     │                         │
    │  Redirect vers     │                     │                         │
    │  /auth/callback    │                     │                         │
    │  ?code=xxx         │                     │                         │
    │  &state=xxx        │                     │                         │
    │────────────────────│                     │                         │
    │                    │  POST /api/auth/     │                         │
    │                    │  oauth/callback      │                         │
    │                    │  { code, state }     │                         │
    │                    │────────────────────▶│                         │
    │                    │                     │  Échange code → token   │
    │                    │                     │────────────────────────▶│
    │                    │                     │  token + infos user     │
    │                    │                     │◀────────────────────────│
    │                    │                     │                         │
    │                    │  JWT LearnLab        │                         │
    │                    │◀────────────────────│                         │
    │  Connecté          │                     │                         │
    │◀───────────────────│                     │                         │
```

**Sécurité CSRF** : le paramètre `state` est un JWT signé avec le `JWT_SECRET` de la plateforme, valide 10 minutes. L'API vérifie sa signature avant d'échanger le code.

**Liaison de compte** : lors de la première connexion SSO, l'API cherche un compte existant dans cet ordre :
1. Correspondance exacte sur `(provider, provider_id)` → reconnexion
2. Correspondance sur l'email → le compte local existant est lié au provider SSO
3. Aucune correspondance → création automatique d'un nouveau compte

---

### Activer GitLab SSO

#### 1. Créer une application OAuth sur GitLab

- **gitlab.com** : Preferences → Applications → Add new application
- **Instance auto-hébergée** : `https://votre-gitlab.com/-/profile/applications`

| Champ | Valeur |
|-------|--------|
| Name | `LearnLab` (ou autre) |
| Redirect URI | `http://votre-domaine.com/auth/callback` |
| Confidential | ✅ coché |
| Scopes | `read_user` |

Copiez le **Application ID** et le **Secret** générés.

#### 2. Configurer les variables d'environnement

Dans `.env` (racine du projet) :

```env
GITLAB_CLIENT_ID=votre_application_id
GITLAB_CLIENT_SECRET=votre_secret
OAUTH_REDIRECT_BASE=http://votre-domaine.com   # sans slash final
```

Pour une instance GitLab **auto-hébergée**, ajoutez également :

```env
GITLAB_URL=https://gitlab.votre-entreprise.com
```

> L'URL peut aussi être modifiée à chaud depuis l'interface admin sans redémarrage du serveur (voir ci-dessous).

#### 3. Redémarrer l'API

```bash
docker compose up -d --build api
```

Le bouton **Continue with GitLab** apparaît automatiquement sur la page de login dès que `GITLAB_CLIENT_ID` est renseigné.

---

### Activer GitHub SSO

#### 1. Créer une OAuth App sur GitHub

GitHub → Settings → Developer settings → **OAuth Apps** → New OAuth App

| Champ | Valeur |
|-------|--------|
| Application name | `LearnLab` |
| Homepage URL | `http://votre-domaine.com` |
| Authorization callback URL | `http://votre-domaine.com/auth/callback` |

Copiez le **Client ID** et générez un **Client secret**.

#### 2. Configurer les variables d'environnement

```env
GITHUB_CLIENT_ID=votre_client_id
GITHUB_CLIENT_SECRET=votre_client_secret
OAUTH_REDIRECT_BASE=http://votre-domaine.com
```

#### 3. Redémarrer l'API

```bash
docker compose up -d --build api
```

---

### Modifier l'URL GitLab depuis l'interface admin

Pour les instances GitLab auto-hébergées dont l'URL peut changer, il n'est pas nécessaire de redémarrer le serveur :

1. Se connecter avec un compte admin
2. Aller dans **Admin → Settings** (icône ⚙️ dans la sidebar)
3. Section **SSO & Authentication** → champ **GitLab Instance URL**
4. Modifier l'URL et cliquer **Save Settings**

La modification est effective immédiatement pour tous les nouveaux flux OAuth.

---

### Paramètres SSO et utilisateurs (Admin → Settings)

Le panneau d'administration expose les réglages suivants, modifiables à chaud :

| Section | Paramètre | Description |
|---------|-----------|-------------|
| **Registration** | Allow public registration | Autorise ou non les nouvelles inscriptions |
| | Email domain whitelist | Restreint l'inscription à certains domaines (ex. `company.com`) |
| **Password Policy** | Minimum length | Longueur minimale du mot de passe |
| | Require uppercase | Au moins une lettre majuscule |
| | Require number | Au moins un chiffre |
| **User Profiles** | Allow username change | Autorise les utilisateurs à modifier leur pseudo |
| **SSO & Auth** | Allow local login | Désactiver pour forcer l'authentification SSO uniquement |
| | GitLab Instance URL | URL de l'instance GitLab (modifiable sans redémarrage) |

> **Attention** : si vous désactivez _Allow local login_ sans avoir au moins un provider SSO configuré, plus personne ne pourra se connecter. L'interface affiche un avertissement dans ce cas.

---

### Comptes SSO — comportement

| Situation | Résultat |
|-----------|----------|
| Premier login SSO, email inconnu | Compte créé automatiquement (rôle `student`) |
| Premier login SSO, email déjà enregistré en local | Le compte local est lié au provider SSO |
| Login suivant via SSO | Reconnexion directe, avatar mis à jour |
| Tentative de login local sur un compte SSO | Erreur : _"This account uses GitLab SSO login"_ |
| Tentative de changement de mot de passe sur un compte SSO | Erreur : _"This account uses SSO login and has no local password"_ |

---

### Endpoints API SSO

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| `GET` | `/api/auth/oauth/providers` | Liste des providers activés |
| `GET` | `/api/auth/oauth/:provider/authorize` | Génère l'URL d'autorisation + state CSRF |
| `POST` | `/api/auth/oauth/callback` | Échange le code → JWT LearnLab |
| `GET` | `/api/settings/public` | Paramètres publics (registration, password policy…) |
| `GET` | `/api/admin/settings` | Tous les paramètres (admin) |
| `PUT` | `/api/admin/settings` | Met à jour les paramètres (admin) |

---

## Quick Start (Docker Compose)

### Premier lancement

```bash
cp .env.example .env
# Modifier .env : changer DB_PASSWORD, JWT_SECRET et GRAFANA_PASSWORD

docker compose up --build -d
```

### Lancer l'app

```bash
docker compose up -d
```

### Éteindre l'app

```bash
# Arrêter sans supprimer les données
docker compose down

# Arrêter ET supprimer les données (DB, Grafana, Prometheus)
docker compose down -v
```

### Rebuild après modification du code

```bash
docker compose build --no-cache api     # rebuild l'API Rust
docker compose build --no-cache frontend  # rebuild le frontend
docker compose up -d
```

| Service    | URL                         |
|------------|-----------------------------|
| Frontend   | http://localhost:3000        |
| API        | http://localhost:8080        |
| Prometheus | http://localhost:9090        |
| Grafana    | http://localhost:3001        |

**Default admin**: `admin@elearning.local` / `Admin@1234`

---

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│  SvelteKit  │───▶│   Rust/Axum  │───▶│  PostgreSQL  │
│  Frontend   │    │     API      │    │              │
│  :3000      │    │  :8080       │    │  :5432       │
└─────────────┘    └──────┬───────┘    └──────────────┘
                          │ /metrics
                   ┌──────▼───────┐
                   │  Prometheus  │───▶ Grafana :3001
                   │  :9090       │
                   └──────────────┘
```

## Lab Types

### 📝 Form Labs (Quiz)
- Multiple choice, text, or code questions
- Per-question scoring with explanations
- Instant feedback with correct answers shown after submission

### 🚩 CTF Challenges (Root-Me style)
- Challenge description + optional hints/resources
- Submit a flag string (`FLAG{...}`)
- Unlimited attempts, best score tracked
- Category tags: web, crypto, forensics, pwn, reverse, misc

## API Endpoints

### Auth
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/register` | Register |
| POST | `/api/auth/login` | Login → JWT |
| GET | `/api/auth/me` | Current user |
| PUT | `/api/auth/password` | Change password |

### Courses
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/courses` | List published (filter: search, difficulty, category) |
| GET | `/api/courses/:id` | Get course |
| POST | `/api/courses` | Create (auth) |
| PUT | `/api/courses/:id` | Update (owner/admin) |
| DELETE | `/api/courses/:id` | Delete (owner/admin) |
| POST | `/api/courses/:id/enroll` | Enroll |
| GET | `/api/my/courses` | My enrolled courses |

### Labs
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/courses/:cid/labs` | List labs (enrolled) |
| GET | `/api/courses/:cid/labs/:lid` | Get lab + my progress |
| POST | `/api/courses/:cid/labs` | Create lab (owner/admin) |
| PUT | `/api/courses/:cid/labs/:lid` | Update lab |
| DELETE | `/api/courses/:cid/labs/:lid` | Delete lab |

### Submissions
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/courses/:cid/labs/:lid/submit` | Submit answer |
| GET | `/api/courses/:cid/labs/:lid/submissions` | My submissions |
| GET | `/api/courses/:cid/progress` | My course progress |

### Admin
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/admin/stats` | Platform stats |
| GET | `/api/admin/users` | All users |
| PUT | `/api/admin/users/:id` | Update user (role, active) |
| DELETE | `/api/admin/users/:id` | Delete user |
| GET | `/api/admin/courses` | All courses (inc. drafts) |
| GET | `/api/admin/courses/:cid/monitoring` | Student progress for course |
| GET | `/api/admin/courses/:cid/labs/:lid/submissions` | All submissions for a lab |

### Monitoring
| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /metrics` | Prometheus metrics |

## Prometheus Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `http_requests_total` | Counter | method, endpoint, status |
| `http_request_duration_seconds` | Histogram | method, endpoint |
| `elearning_active_users_total` | Gauge | — |
| `elearning_active_courses_total` | Gauge | — |
| `elearning_enrollments_total` | Gauge | — |
| `elearning_lab_submissions_total` | Counter | lab_type, result |

## Kubernetes / Helm

```bash
# Add bitnami repo for PostgreSQL
helm repo add bitnami https://charts.bitnami.com/bitnami
helm dependency update ./helm

# Build Docker images
docker build -t elearning-api:latest ./api
docker build -t elearning-frontend:latest ./frontend

# Push to your registry
docker tag elearning-api:latest registry.example.com/elearning-api:latest
docker push registry.example.com/elearning-api:latest

# Install
helm install elearning ./helm \
  --namespace elearning \
  --create-namespace \
  --set api.image.repository=registry.example.com/elearning-api \
  --set frontend.image.repository=registry.example.com/elearning-frontend \
  --set api.jwtSecret="your-long-secret-here" \
  --set ingress.hosts[0].host=elearning.yourdomain.com

# With kube-prometheus-stack (ServiceMonitor)
helm install elearning ./helm \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.labels.release=kube-prometheus-stack
```

## Local Development

### API
```bash
cd api
cp ../.env.example .env  # edit DATABASE_URL
cargo run
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

## Form Lab JSON Schema

```json
{
  "questions": [
    {
      "id": "q1",
      "text": "What does HTTP stand for?",
      "type": "multiple_choice",
      "options": ["HyperText Transfer Protocol", "High-Tech Transfer Protocol", "Hyper Transfer Text Protocol"],
      "correct_answer": "HyperText Transfer Protocol",
      "points": 25,
      "explanation": "HTTP = HyperText Transfer Protocol, the foundation of the web."
    },
    {
      "id": "q2",
      "text": "What port does HTTPS use by default?",
      "type": "text",
      "correct_answer": "443",
      "points": 25,
      "explanation": "HTTPS uses port 443 (HTTP uses 80)."
    }
  ]
}
```

## CTF Lab JSON Schema

```json
{
  "challenge": "A website stores passwords in plaintext in the URL. Find the admin password by inspecting the login page source code. The flag is in the format FLAG{password}.",
  "category": "web",
  "hints": ["Try viewing the page source", "Look for hidden form fields"],
  "resources": [
    {"name": "Challenge URL", "url": "http://challenge.local:8888"}
  ],
  "docker_image": "optional-challenge-image:latest"
}
```
