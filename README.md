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
