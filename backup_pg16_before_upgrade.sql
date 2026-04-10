--
-- PostgreSQL database cluster dump
--

\restrict aaOMaOmXj1dVHXfx6XpbavoXcVFUdzpJJB8skgzXtO8s57lKcZY2qsZTofWpkPr

SET default_transaction_read_only = off;

SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;

--
-- Roles
--

CREATE ROLE elearning;
ALTER ROLE elearning WITH SUPERUSER INHERIT CREATEROLE CREATEDB LOGIN REPLICATION BYPASSRLS PASSWORD 'SCRAM-SHA-256$4096:DGyjamTc8p4homAjXHV3nw==$XMmfYfVZW6CxJMdmrxJTHLbeBaax7tpCxfGP1Zlv0UM=:1gl4Wd2OtFb8npipmEPxok6o4IVyz11fAZ3HfpQP/FY=';

--
-- User Configurations
--








\unrestrict aaOMaOmXj1dVHXfx6XpbavoXcVFUdzpJJB8skgzXtO8s57lKcZY2qsZTofWpkPr

--
-- Databases
--

--
-- Database "template1" dump
--

\connect template1

--
-- PostgreSQL database dump
--

\restrict r6uXcFu4VelhuPyvYIjZwOdbCijT5jz1A2tqIHblsogQf51bXibjhmR0fMmdCVg

-- Dumped from database version 16.13
-- Dumped by pg_dump version 16.13

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- PostgreSQL database dump complete
--

\unrestrict r6uXcFu4VelhuPyvYIjZwOdbCijT5jz1A2tqIHblsogQf51bXibjhmR0fMmdCVg

--
-- Database "elearning" dump
--

--
-- PostgreSQL database dump
--

\restrict HeXjsQP7jJC404T2TAypFFPMqougH3tmUuc0w6uh8emuBhNffKQMwTPgPnBQ3sL

-- Dumped from database version 16.13
-- Dumped by pg_dump version 16.13

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: elearning; Type: DATABASE; Schema: -; Owner: elearning
--

CREATE DATABASE elearning WITH TEMPLATE = template0 ENCODING = 'UTF8' LOCALE_PROVIDER = libc LOCALE = 'en_US.utf8';


ALTER DATABASE elearning OWNER TO elearning;

\unrestrict HeXjsQP7jJC404T2TAypFFPMqougH3tmUuc0w6uh8emuBhNffKQMwTPgPnBQ3sL
\connect elearning
\restrict HeXjsQP7jJC404T2TAypFFPMqougH3tmUuc0w6uh8emuBhNffKQMwTPgPnBQ3sL

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: _sqlx_migrations; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public._sqlx_migrations (
    version bigint NOT NULL,
    description text NOT NULL,
    installed_on timestamp with time zone DEFAULT now() NOT NULL,
    success boolean NOT NULL,
    checksum bytea NOT NULL,
    execution_time bigint NOT NULL
);


ALTER TABLE public._sqlx_migrations OWNER TO elearning;

--
-- Name: courses; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.courses (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title character varying(255) NOT NULL,
    description text NOT NULL,
    thumbnail text,
    category character varying(64),
    difficulty character varying(16),
    is_published boolean DEFAULT false NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT courses_difficulty_check CHECK (((difficulty)::text = ANY ((ARRAY['beginner'::character varying, 'intermediate'::character varying, 'advanced'::character varying])::text[])))
);


ALTER TABLE public.courses OWNER TO elearning;

--
-- Name: enrollments; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.enrollments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    course_id uuid NOT NULL,
    enrolled_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.enrollments OWNER TO elearning;

--
-- Name: lab_progress; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.lab_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    lab_id uuid NOT NULL,
    course_id uuid NOT NULL,
    completed boolean DEFAULT false NOT NULL,
    best_score integer DEFAULT 0 NOT NULL,
    total_attempts integer DEFAULT 0 NOT NULL,
    completed_at timestamp with time zone,
    last_attempt_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.lab_progress OWNER TO elearning;

--
-- Name: lab_submissions; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.lab_submissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    lab_id uuid NOT NULL,
    user_id uuid NOT NULL,
    answer jsonb NOT NULL,
    is_correct boolean DEFAULT false NOT NULL,
    score integer DEFAULT 0 NOT NULL,
    attempts integer DEFAULT 1 NOT NULL,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.lab_submissions OWNER TO elearning;

--
-- Name: labs; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.labs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    course_id uuid NOT NULL,
    title character varying(255) NOT NULL,
    description text NOT NULL,
    lab_type character varying(16) NOT NULL,
    content jsonb DEFAULT '{}'::jsonb NOT NULL,
    flag text,
    points integer DEFAULT 100 NOT NULL,
    order_index integer DEFAULT 0 NOT NULL,
    is_published boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT labs_lab_type_check CHECK (((lab_type)::text = ANY ((ARRAY['form'::character varying, 'ctf'::character varying])::text[])))
);


ALTER TABLE public.labs OWNER TO elearning;

--
-- Name: users; Type: TABLE; Schema: public; Owner: elearning
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    username character varying(64) NOT NULL,
    email character varying(255) NOT NULL,
    password_hash text NOT NULL,
    role character varying(16) DEFAULT 'student'::character varying NOT NULL,
    avatar_url text,
    bio text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT users_role_check CHECK (((role)::text = ANY ((ARRAY['admin'::character varying, 'student'::character varying])::text[])))
);


ALTER TABLE public.users OWNER TO elearning;

--
-- Data for Name: _sqlx_migrations; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public._sqlx_migrations (version, description, installed_on, success, checksum, execution_time) FROM stdin;
1	init	2026-04-06 09:05:06.960017+00	t	\\xe78aeae6c0be3db92d3c420869d04a8badfbdf54c7acbf197d6970b4ba63ca33823ee018b8ad88d65a2d41a642866242	248699599
2	fix admin	2026-04-06 09:05:07.211574+00	t	\\xdb58da80a15e3175905e53164acbd395ca83247012686e6ce93b120ee41a261746e7c5768f854ca7968e8e2810818d30	2880498
\.


--
-- Data for Name: courses; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.courses (id, title, description, thumbnail, category, difficulty, is_published, created_by, created_at, updated_at) FROM stdin;
741c1b16-5400-4c79-a40a-8ed5c10b1620	DevOps Fondamentaux	Maîtrisez les principes fondamentaux du DevOps : culture, pratiques et outils. Apprenez à casser les silos entre Dev et Ops pour livrer plus vite et plus fiable.	\N	DevOps	beginner	t	a1577f15-2756-4271-b96e-d2ed098de544	2026-04-06 15:39:35.589013+00	2026-04-06 15:39:35.589013+00
70766af7-5071-4356-a023-32c34b867ed8	CI/CD avec GitHub Actions	Automatisez vos pipelines de build, test et déploiement avec GitHub Actions. De zéro à la production en passant par des workflows avancés.	\N	CI/CD	intermediate	t	a1577f15-2756-4271-b96e-d2ed098de544	2026-04-06 15:39:35.610831+00	2026-04-06 15:39:35.610831+00
f370f937-493e-4676-8a69-d320fd3fed80	GitOps avec ArgoCD	Gérez vos déploiements Kubernetes avec le paradigme GitOps. Git comme source de vérité, ArgoCD pour la synchronisation automatique de vos environnements.	\N	GitOps	advanced	t	a1577f15-2756-4271-b96e-d2ed098de544	2026-04-06 15:39:35.623678+00	2026-04-06 15:39:35.623678+00
b7f7d77d-6d51-4603-b73d-74b0683196f5	Test		\N	ezofoi	beginner	t	a1577f15-2756-4271-b96e-d2ed098de544	2026-04-07 15:50:10.72878+00	2026-04-07 15:50:10.72878+00
\.


--
-- Data for Name: enrollments; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.enrollments (id, user_id, course_id, enrolled_at) FROM stdin;
8b2d209d-9cb4-428d-bdd4-74146904264a	1b450d6a-f657-4c95-acfa-75bc7a6511de	f370f937-493e-4676-8a69-d320fd3fed80	2026-04-06 15:55:48.331487+00
9300f9e3-ba8c-4cc9-9afa-3f6a7d97ed35	2750cc1f-6cbe-4f5d-ac9d-65325cf98281	70766af7-5071-4356-a023-32c34b867ed8	2026-04-06 15:55:53.823372+00
18eddc00-85e6-43ef-9a6f-180bd060274a	2750cc1f-6cbe-4f5d-ac9d-65325cf98281	741c1b16-5400-4c79-a40a-8ed5c10b1620	2026-04-06 15:55:57.858407+00
f111186e-0fb3-4fe4-a97d-f07f81b1e975	a1577f15-2756-4271-b96e-d2ed098de544	f370f937-493e-4676-8a69-d320fd3fed80	2026-04-06 17:53:12.306447+00
\.


--
-- Data for Name: lab_progress; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.lab_progress (id, user_id, lab_id, course_id, completed, best_score, total_attempts, completed_at, last_attempt_at) FROM stdin;
6111c266-0bc2-45cb-8544-30eff3e4de64	a1577f15-2756-4271-b96e-d2ed098de544	04e2537b-c6e4-44e4-911b-8b8ba2dc3f0e	f370f937-493e-4676-8a69-d320fd3fed80	f	250	1	\N	2026-04-06 17:53:12.3464+00
f1dea224-0ad0-4e2f-899e-756f6943531d	1b450d6a-f657-4c95-acfa-75bc7a6511de	15a187a7-60c2-42aa-8950-7963ac5be4a7	f370f937-493e-4676-8a69-d320fd3fed80	t	300	1	2026-04-06 18:54:08.471504+00	2026-04-06 18:54:08.472428+00
a0ac2ba3-8ded-4d38-b273-8e9a5f6cb455	1b450d6a-f657-4c95-acfa-75bc7a6511de	a9f18d8c-92b2-4fb7-9f76-e0c272418418	f370f937-493e-4676-8a69-d320fd3fed80	f	75	1	\N	2026-04-07 15:49:25.226356+00
\.


--
-- Data for Name: lab_submissions; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.lab_submissions (id, lab_id, user_id, answer, is_correct, score, attempts, submitted_at) FROM stdin;
39913c93-74a3-449d-82c0-d756368a12de	04e2537b-c6e4-44e4-911b-8b8ba2dc3f0e	a1577f15-2756-4271-b96e-d2ed098de544	{"flags": {"lfi": "WRONG_FLAG", "rce": "FLAG{blind_ssrf_metadata_endpoint}", "root": "", "recon": "FLAG{nmap_found_port_8888}"}}	f	250	1	2026-04-06 17:53:12.342829+00
a9ffd781-18b8-4bfd-a422-7e58cc85c6dc	15a187a7-60c2-42aa-8950-7963ac5be4a7	1b450d6a-f657-4c95-acfa-75bc7a6511de	{"flag": "FLAG{base64_is_not_encryption}"}	t	300	1	2026-04-06 18:54:08.460315+00
b61afa79-4bec-4433-a5e7-66c71cc70ac6	a9f18d8c-92b2-4fb7-9f76-e0c272418418	1b450d6a-f657-4c95-acfa-75bc7a6511de	{"answers": {"q1": "Le dépôt Git contenant les manifests", "q2": "Dans le modèle Pull, c'est l'agent dans le cluster qui tire les changements depuis Git, pas le pipeline CI/CD qui pousse", "q3": "L'agent détecte les divergences entre l'état Git et l'état cluster, et les corrige automatiquement", "q4": "Cela réduit le coût du cluster"}}	f	75	1	2026-04-07 15:49:25.218679+00
\.


--
-- Data for Name: labs; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.labs (id, course_id, title, description, lab_type, content, flag, points, order_index, is_published, created_at, updated_at) FROM stdin;
c946829e-1cc8-4149-95b6-2a26ccb8c48e	741c1b16-5400-4c79-a40a-8ed5c10b1620	Qu'est-ce que le DevOps ?	Introduction aux principes fondamentaux du DevOps et à la culture CALMS.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 25, "options": ["Culture, Automation, Lean, Metrics, Sharing", "Continuous, Agile, Learning, Monitoring, Security", "Cloud, API, Linux, Microservices, Scalability", "Collaboration, Agility, Logging, Management, Speed"], "question": "Que signifie l'acronyme CALMS dans le contexte du DevOps ?", "correct_answer": "Culture, Automation, Lean, Metrics, Sharing"}, {"id": "q2", "type": "multiple_choice", "points": 25, "options": ["Remplacer les administrateurs système par des développeurs", "Briser les silos entre Dev et Ops pour livrer plus vite et de façon plus fiable", "Automatiser 100% des tâches manuelles", "Migrer toutes les applications vers le cloud"], "question": "Quel est le principal objectif du mouvement DevOps ?", "correct_answer": "Briser les silos entre Dev et Ops pour livrer plus vite et de façon plus fiable"}, {"id": "q3", "type": "multiple_choice", "points": 25, "options": ["Intégration Continue (CI)", "Infrastructure as Code (IaC)", "Déploiement manuel hebdomadaire", "Monitoring et observabilité"], "question": "Parmi les pratiques suivantes, laquelle N'est PAS une pratique DevOps typique ?", "correct_answer": "Déploiement manuel hebdomadaire"}, {"id": "q4", "type": "multiple_choice", "points": 25, "options": ["La Fiabilité", "L'Apprentissage Continu", "La Sécurité", "La Scalabilité"], "question": "Le concept de 'Three Ways' du DevOps comprend : le Flow, le Feedback et...", "correct_answer": "L'Apprentissage Continu"}], "instructions": "Répondez aux questions suivantes sur les fondamentaux du DevOps. Le mouvement DevOps est né de la convergence entre les équipes de développement (Dev) et d'opérations (Ops), avec pour objectif de briser les silos organisationnels."}	\N	100	0	t	2026-04-06 15:50:39.90593+00	2026-04-06 15:50:39.90593+00
aa97d186-314a-4964-acf2-33c7071e2773	741c1b16-5400-4c79-a40a-8ed5c10b1620	Les 4 métriques DORA	Maîtrisez les métriques DORA pour mesurer la performance DevOps de votre équipe.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 10, "options": ["3", "4", "5", "6"], "question": "Combien de métriques DORA existe-t-il ?", "correct_answer": "4"}, {"id": "q2", "type": "multiple_choice", "points": 25, "options": ["Le nombre de bugs par déploiement", "La fréquence à laquelle une équipe déploie du code en production", "Le nombre de développeurs dans l'équipe", "La taille des releases"], "question": "Que mesure le 'Deployment Frequency' (fréquence de déploiement) ?", "correct_answer": "La fréquence à laquelle une équipe déploie du code en production"}, {"id": "q3", "type": "multiple_choice", "points": 25, "options": ["Le temps pour déployer une hotfix en urgence", "Le temps entre un commit et son déploiement en production", "La durée moyenne d'un sprint", "Le temps de build d'un pipeline CI"], "question": "Le 'Lead Time for Changes' mesure :", "correct_answer": "Le temps entre un commit et son déploiement en production"}, {"id": "q4", "type": "multiple_choice", "points": 25, "options": ["Le temps pour développer une nouvelle fonctionnalité", "Le temps moyen pour restaurer le service après un incident en production", "Le délai entre deux releases majeures", "Le temps de réponse moyen de l'API"], "question": "Qu'est-ce que le 'Time to Restore Service' (MTTR) ?", "correct_answer": "Le temps moyen pour restaurer le service après un incident en production"}, {"id": "q5", "type": "multiple_choice", "points": 15, "options": ["Le pourcentage de développeurs quittant l'équipe", "Le pourcentage de déploiements causant un incident ou nécessitant un rollback", "Le taux d'échec des builds CI", "Le nombre de features annulées"], "question": "Le 'Change Failure Rate' représente :", "correct_answer": "Le pourcentage de déploiements causant un incident ou nécessitant un rollback"}], "instructions": "Les métriques DORA (DevOps Research and Assessment) sont les 4 indicateurs clés pour mesurer la performance d'une équipe de livraison logicielle. Répondez aux questions suivantes."}	\N	100	1	t	2026-04-06 15:50:39.910316+00	2026-04-06 15:50:39.910316+00
47288603-c7d6-4afa-ab25-cc6c5c757566	741c1b16-5400-4c79-a40a-8ed5c10b1620	Infrastructure as Code (IaC)	Comprenez les principes de l'Infrastructure as Code et les outils associés.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 30, "options": ["Réduire le coût des serveurs physiques", "Rendre l'infrastructure reproductible, versionnée et automatisable", "Supprimer le besoin d'administrateurs système", "Permettre le déploiement uniquement sur AWS"], "question": "Quel est le principal avantage de l'Infrastructure as Code ?", "correct_answer": "Rendre l'infrastructure reproductible, versionnée et automatisable"}, {"id": "q2", "type": "multiple_choice", "points": 30, "options": ["Impérative (comment faire)", "Déclarative (quoi faire)", "Procédurale (étape par étape)", "Réactive (basée sur les événements)"], "question": "Terraform utilise quel type d'approche pour gérer l'infrastructure ?", "correct_answer": "Déclarative (quoi faire)"}, {"id": "q3", "type": "multiple_choice", "points": 30, "options": ["terraform run", "terraform deploy", "terraform apply", "terraform push"], "question": "Quelle commande Terraform permet d'appliquer les changements d'infrastructure ?", "correct_answer": "terraform apply"}, {"id": "q4", "type": "multiple_choice", "points": 30, "options": ["Il est uniquement pour le cloud AWS", "Il est agentless et utilise SSH pour la configuration", "Il nécessite un agent sur chaque machine cible", "Il ne supporte que les environnements Linux"], "question": "Ansible est un outil d'IaC. Quelle est sa principale caractéristique par rapport à Terraform ?", "correct_answer": "Il est agentless et utilise SSH pour la configuration"}, {"id": "q5", "type": "multiple_choice", "points": 30, "options": ["L'état de santé des serveurs", "Un fichier qui mappe la configuration Terraform aux ressources réelles", "Les logs d'exécution des playbooks", "Le statut du pipeline CI/CD"], "question": "Qu'est-ce que le 'state' dans Terraform ?", "correct_answer": "Un fichier qui mappe la configuration Terraform aux ressources réelles"}], "instructions": "L'Infrastructure as Code (IaC) permet de gérer et provisionner l'infrastructure via des fichiers de configuration versionnés, plutôt que des processus manuels. Répondez aux questions suivantes."}	\N	150	2	t	2026-04-06 15:50:39.914101+00	2026-04-06 15:50:39.914101+00
055db86e-e022-4a6f-bfa8-d73648901daf	741c1b16-5400-4c79-a40a-8ed5c10b1620	CTF: Ne commitez jamais vos secrets !	Un développeur a accidentellement commité des credentials dans un dépôt Git. Retrouvez le flag caché dans l'historique.	ctf	{"flag_hint": "Le flag est dans le fichier .env montré ci-dessus, dans la variable CI_FLAG", "instructions": "## Mission\\n\\nUn développeur junior a poussé du code contenant des secrets sur GitHub. Le repo a depuis été rendu public par erreur.\\n\\nVoici un extrait du fichier `.env` committé par erreur :\\n\\n```\\nDB_HOST=prod-db.internal\\nDB_USER=admin\\nDB_PASSWORD=Sup3rS3cr3t!\\nAPI_KEY=sk-prod-123456789\\nCI_FLAG=FLAG{never_commit_secrets_in_ci}\\nSTRIPE_SECRET=sk_live_abc123\\n```\\n\\nLe développeur a tenté de supprimer le fichier dans un commit suivant, mais l'historique Git conserve tout.\\n\\n**Leçon**: Utilisez toujours `.gitignore` pour exclure les fichiers `.env`. En cas de fuite, révoquez immédiatement les credentials exposés — supprimer le fichier ne suffit pas.\\n\\nEntrez le flag que vous avez trouvé dans le fichier `.env` ci-dessus pour valider votre compréhension."}	FLAG{never_commit_secrets_in_ci}	200	3	t	2026-04-06 15:50:39.917373+00	2026-04-06 15:50:39.917373+00
4586b40a-caa0-45df-9ca4-0cf2372621c4	70766af7-5071-4356-a023-32c34b867ed8	Anatomie d'un workflow GitHub Actions	Comprenez la structure et les composants d'un fichier YAML de workflow GitHub Actions.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 20, "options": [".github/actions/", ".github/workflows/", "ci/workflows/", ".actions/"], "question": "Dans quel répertoire doit se trouver un fichier de workflow GitHub Actions ?", "correct_answer": ".github/workflows/"}, {"id": "q2", "type": "multiple_choice", "points": 20, "options": ["Uniquement sur les push vers main", "Sur les push vers main ET les pull requests vers main", "Sur tous les push peu importe la branche", "Uniquement sur les pull requests"], "question": "Dans le workflow ci-dessus, sur quel événement le pipeline se déclenche-t-il ?", "correct_answer": "Sur les push vers main ET les pull requests vers main"}, {"id": "q3", "type": "multiple_choice", "points": 20, "options": ["Installe Node.js version 4", "Clone le dépôt dans le runner pour accéder au code source", "Vérifie la syntaxe du fichier YAML", "Publie le code sur GitHub Pages"], "question": "Que fait l'étape 'uses: actions/checkout@v4' ?", "correct_answer": "Clone le dépôt dans le runner pour accéder au code source"}, {"id": "q4", "type": "multiple_choice", "points": 20, "options": ["'run' exécute une commande shell, 'uses' réutilise une action GitHub", "'run' est pour les tests, 'uses' est pour le déploiement", "Il n'y a pas de différence", "'run' est obsolète, il faut utiliser 'uses'"], "question": "Quelle est la différence entre 'run' et 'uses' dans une étape ?", "correct_answer": "'run' exécute une commande shell, 'uses' réutilise une action GitHub"}, {"id": "q5", "type": "multiple_choice", "points": 20, "options": ["Le job tourne sur le dernier serveur Ubuntu de l'équipe", "Le job tourne sur un runner GitHub hébergé avec Ubuntu", "Le job nécessite Ubuntu 22.04 exactement", "Le job est exécuté en dernier dans le pipeline"], "question": "Que signifie 'runs-on: ubuntu-latest' ?", "correct_answer": "Le job tourne sur un runner GitHub hébergé avec Ubuntu"}], "instructions": "Un workflow GitHub Actions est un fichier YAML placé dans `.github/workflows/`. Voici un exemple :\\n\\n```yaml\\nname: CI Pipeline\\n\\non:\\n  push:\\n    branches: [main]\\n  pull_request:\\n    branches: [main]\\n\\njobs:\\n  build:\\n    runs-on: ubuntu-latest\\n    steps:\\n      - uses: actions/checkout@v4\\n      - name: Setup Node.js\\n        uses: actions/setup-node@v4\\n        with:\\n          node-version: '20'\\n      - name: Install dependencies\\n        run: npm ci\\n      - name: Run tests\\n        run: npm test\\n```\\n\\nRépondez aux questions basées sur ce workflow."}	\N	100	0	t	2026-04-06 15:50:39.920925+00	2026-04-06 15:50:39.920925+00
c9923ff2-d5de-4d01-8989-f2969d7a394f	70766af7-5071-4356-a023-32c34b867ed8	Pipeline CI : Build et Test	Construisez mentalement un pipeline CI complet avec étapes de build, test et qualité de code.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 30, "options": ["Définit l'ordre d'exécution des jobs", "Lance le job en parallèle avec différentes versions de Node.js", "Crée une matrice de permissions", "Configure les environnements de déploiement"], "question": "Que fait la section 'strategy.matrix' dans ce workflow ?", "correct_answer": "Lance le job en parallèle avec différentes versions de Node.js"}, {"id": "q2", "type": "multiple_choice", "points": 20, "options": ["1", "2", "3", "6"], "question": "Combien de jobs seront créés avec la matrice node-version: [18, 20, 22] ?", "correct_answer": "3"}, {"id": "q3", "type": "multiple_choice", "points": 30, "options": ["npm ci est plus rapide car il ne résout pas les dépendances", "npm ci installe exactement les versions du package-lock.json et échoue si le lock est désynchronisé", "npm ci ne nécessite pas internet", "npm ci installe aussi les devDependencies"], "question": "Pourquoi utilise-t-on 'npm ci' plutôt que 'npm install' en CI ?", "correct_answer": "npm ci installe exactement les versions du package-lock.json et échoue si le lock est désynchronisé"}, {"id": "q4", "type": "multiple_choice", "points": 30, "options": ["Accélérer les builds suivants", "Suivre l'évolution de la couverture de tests dans le temps et sur chaque PR", "Stocker une sauvegarde du code", "Déployer automatiquement en production"], "question": "Quel est l'intérêt d'uploader la couverture de code (coverage) vers un service comme Codecov ?", "correct_answer": "Suivre l'évolution de la couverture de tests dans le temps et sur chaque PR"}, {"id": "q5", "type": "multiple_choice", "points": 40, "options": ["Nettoyer les fichiers temporaires", "Analyser statiquement le code pour détecter erreurs de style et bugs potentiels", "Vérifier la disponibilité des dépendances", "Générer la documentation"], "question": "Le 'lint' dans un pipeline CI sert à :", "correct_answer": "Analyser statiquement le code pour détecter erreurs de style et bugs potentiels"}], "instructions": "L'Intégration Continue (CI) consiste à vérifier automatiquement chaque changement de code. Un pipeline CI typique comprend : lint → build → test → security scan.\\n\\nVoici un pipeline plus avancé avec une matrice de tests :\\n\\n```yaml\\njobs:\\n  test:\\n    runs-on: ubuntu-latest\\n    strategy:\\n      matrix:\\n        node-version: [18, 20, 22]\\n    steps:\\n      - uses: actions/checkout@v4\\n      - uses: actions/setup-node@v4\\n        with:\\n          node-version: ${{ matrix.node-version }}\\n      - run: npm ci\\n      - run: npm run lint\\n      - run: npm test -- --coverage\\n      - name: Upload coverage\\n        uses: codecov/codecov-action@v4\\n```\\n\\nRépondez aux questions suivantes."}	\N	150	1	t	2026-04-06 15:50:39.924552+00	2026-04-06 15:50:39.924552+00
915eb257-bddd-43bf-b69f-28e9d9f24e61	70766af7-5071-4356-a023-32c34b867ed8	Déploiement Continu et Environnements	Maîtrisez les stratégies de déploiement : Blue/Green, Canary, et la gestion des environnements.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 30, "options": ["Il faut redéployer la version précédente, ce qui prend du temps", "On repointe le load balancer vers l'environnement Blue (rollback instantané)", "L'application est indisponible jusqu'à correction du bug", "Le système se répare automatiquement"], "question": "Dans une stratégie Blue/Green, que se passe-t-il en cas de problème après le basculement ?", "correct_answer": "On repointe le load balancer vers l'environnement Blue (rollback instantané)"}, {"id": "q2", "type": "multiple_choice", "points": 30, "options": ["Il est moins cher en infrastructure", "Il permet de tester en production sur un sous-ensemble d'utilisateurs avant déploiement total", "Il supprime le besoin de tests automatisés", "Il déploie toujours en moins de 1 minute"], "question": "Quel est l'avantage principal d'un déploiement Canary ?", "correct_answer": "Il permet de tester en production sur un sous-ensemble d'utilisateurs avant déploiement total"}, {"id": "q3", "type": "multiple_choice", "points": 30, "options": ["Copie les artifacts du job staging vers production", "Le job production ne s'exécute que si deploy-staging a réussi", "Lance les deux jobs en parallèle", "Partage les variables d'environnement entre jobs"], "question": "Dans le workflow ci-dessus, que fait 'needs: deploy-staging' ?", "correct_answer": "Le job production ne s'exécute que si deploy-staging a réussi"}, {"id": "q4", "type": "multiple_choice", "points": 30, "options": ["Définir les variables d'environnement système", "Configurer des règles de protection, approbations manuelles et secrets par environnement", "Isoler les runners dans des conteneurs séparés", "Définir les branches autorisées à merger"], "question": "À quoi sert la notion d'Environment dans GitHub Actions ?", "correct_answer": "Configurer des règles de protection, approbations manuelles et secrets par environnement"}, {"id": "q5", "type": "multiple_choice", "points": 30, "options": ["Aucune différence, ce sont des synonymes", "Continuous Delivery requiert une approbation manuelle avant prod, Continuous Deployment déploie automatiquement", "Continuous Deployment est plus lent", "Continuous Delivery ne fait que builder, pas déployer"], "question": "Quelle est la différence entre Continuous Delivery et Continuous Deployment ?", "correct_answer": "Continuous Delivery requiert une approbation manuelle avant prod, Continuous Deployment déploie automatiquement"}], "instructions": "Le Déploiement Continu (CD) automatise la mise en production après validation CI. Différentes stratégies permettent de minimiser les risques.\\n\\n**Blue/Green Deployment** : Deux environnements identiques (Blue=actuel, Green=nouveau). On bascule le trafic vers Green une fois validé. Rollback immédiat possible.\\n\\n**Canary Deployment** : On envoie d'abord 5-10% du trafic vers la nouvelle version, on monitore, puis on étend progressivement.\\n\\n**Rolling Deployment** : On remplace les instances progressivement (ex: 25% à la fois).\\n\\nVoici un exemple de déploiement avec environnements dans GitHub Actions :\\n\\n```yaml\\njobs:\\n  deploy-staging:\\n    environment: staging\\n    runs-on: ubuntu-latest\\n    steps:\\n      - run: echo \\"Deploy to staging\\"\\n\\n  deploy-production:\\n    needs: deploy-staging\\n    environment:\\n      name: production\\n      url: https://app.example.com\\n    runs-on: ubuntu-latest\\n    steps:\\n      - run: echo \\"Deploy to production\\"\\n```"}	\N	150	2	t	2026-04-06 15:50:39.928003+00	2026-04-06 15:50:39.928003+00
190bbdc8-a032-4b9b-99bf-6d007765a664	70766af7-5071-4356-a023-32c34b867ed8	CTF : Corriger le pipeline cassé	Un pipeline GitHub Actions est cassé. Identifiez le problème pour récupérer le flag.	ctf	{"flag_hint": "Le flag correspond à la clé YAML qui manquait pour déclencher le pipeline sur les pull requests", "instructions": "## Mission : Réparer le pipeline\\n\\nVotre équipe a un pipeline GitHub Actions qui ne se déclenche jamais sur les Pull Requests. Voici le fichier `.github/workflows/ci.yml` problématique :\\n\\n```yaml\\nname: CI\\n\\non:\\n  push:\\n    branches: [main]\\n\\njobs:\\n  test:\\n    runs-on: ubuntu-latest\\n    steps:\\n      - uses: actions/checkout@v4\\n      - run: npm test\\n```\\n\\nEt voici la version corrigée qui se déclenche aussi sur les Pull Requests :\\n\\n```yaml\\nname: CI\\n\\non:\\n  push:\\n    branches: [main]\\n  pull_request:\\n    branches: [main]\\n\\njobs:\\n  test:\\n    runs-on: ubuntu-latest\\n    steps:\\n      - uses: actions/checkout@v4\\n      - run: npm test\\n```\\n\\n**Quelle ligne a été ajoutée pour déclencher le pipeline sur les PR ?**\\n\\nLe flag est : `FLAG{pull_request_trigger}`\\n\\nEntrez le flag pour valider votre compréhension de la syntaxe des triggers GitHub Actions."}	FLAG{pull_request_trigger}	250	3	t	2026-04-06 15:50:39.932784+00	2026-04-06 15:50:39.932784+00
a9f18d8c-92b2-4fb7-9f76-e0c272418418	f370f937-493e-4676-8a69-d320fd3fed80	Principes du GitOps	Comprenez les 4 principes fondamentaux du GitOps et pourquoi Git devient la source de vérité.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 25, "options": ["La base de données de production", "Le cluster Kubernetes", "Le dépôt Git contenant les manifests", "Le registre de conteneurs Docker"], "question": "Dans le modèle GitOps, quelle est la 'source de vérité' ?", "correct_answer": "Le dépôt Git contenant les manifests"}, {"id": "q2", "type": "multiple_choice", "points": 25, "options": ["Le modèle Push est plus sécurisé", "Dans le modèle Pull, c'est l'agent dans le cluster qui tire les changements depuis Git, pas le pipeline CI/CD qui pousse", "Le modèle Pull nécessite plus de permissions", "Il n'y a aucune différence pratique"], "question": "Quelle est la différence principale entre le modèle Push et le modèle Pull en GitOps ?", "correct_answer": "Dans le modèle Pull, c'est l'agent dans le cluster qui tire les changements depuis Git, pas le pipeline CI/CD qui pousse"}, {"id": "q3", "type": "multiple_choice", "points": 25, "options": ["Le processus de merge des pull requests", "L'agent détecte les divergences entre l'état Git et l'état cluster, et les corrige automatiquement", "La synchronisation des secrets entre environnements", "Le backup automatique des données"], "question": "Qu'est-ce que la 'réconciliation' dans le contexte GitOps ?", "correct_answer": "L'agent détecte les divergences entre l'état Git et l'état cluster, et les corrige automatiquement"}, {"id": "q4", "type": "multiple_choice", "points": 25, "options": ["Les déploiements sont plus rapides", "On obtient un audit trail complet, la possibilité de rollback et une révision collaborative via PR", "Cela réduit le coût du cluster", "Les applications démarrent plus vite"], "question": "Quel est l'avantage de stocker l'état désiré dans Git plutôt que de l'appliquer manuellement ?", "correct_answer": "On obtient un audit trail complet, la possibilité de rollback et une révision collaborative via PR"}], "instructions": "Le **GitOps** est une approche opérationnelle où Git est la source de vérité unique pour l'infrastructure et les applications. Les 4 principes GitOps (OpenGitOps) sont :\\n\\n1. **Déclaratif** : L'état désiré du système est exprimé de façon déclarative\\n2. **Versionné et immutable** : L'état désiré est stocké dans Git (historique, audit, rollback)\\n3. **Retiré automatiquement** : Les agents approuvent et appliquent automatiquement l'état désiré (pull model)\\n4. **Réconcilié en continu** : Les agents observent l'état actuel et le corrigent si il diverge\\n\\n**Pull model vs Push model** :\\n- Push : Le pipeline CI/CD pousse les changements vers le cluster (kubectl apply dans GitHub Actions)\\n- Pull : Un agent dans le cluster tire les changements depuis Git (ArgoCD, Flux)\\n\\nRépondez aux questions suivantes."}	\N	100	0	t	2026-04-06 15:50:39.937598+00	2026-04-06 15:50:39.937598+00
47268d80-d6ef-450c-b80a-3fd43aaf6027	f370f937-493e-4676-8a69-d320fd3fed80	ArgoCD : Concepts et Architecture	Découvrez ArgoCD, l'outil GitOps de référence pour Kubernetes, et ses composants principaux.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 30, "options": ["Répare automatiquement les bugs applicatifs", "Resynchronise automatiquement si quelqu'un modifie le cluster manuellement (drift)", "Redémarre les pods qui crashent", "Génère des alertes en cas de problème"], "question": "Que fait 'syncPolicy.automated.selfHeal: true' dans ArgoCD ?", "correct_answer": "Resynchronise automatiquement si quelqu'un modifie le cluster manuellement (drift)"}, {"id": "q2", "type": "multiple_choice", "points": 30, "options": ["Supprime les logs anciens", "Supprime automatiquement les ressources Kubernetes qui n'existent plus dans Git", "Optimise les images Docker", "Archive les anciennes révisions"], "question": "Que signifie 'prune: true' dans la syncPolicy ArgoCD ?", "correct_answer": "Supprime automatiquement les ressources Kubernetes qui n'existent plus dans Git"}, {"id": "q3", "type": "multiple_choice", "points": 30, "options": ["Il compare les images Docker dans le registry", "Il compare l'état des ressources dans le cluster avec les manifests dans Git", "Il lit les logs des applications", "Il vérifie les health checks HTTP"], "question": "Comment ArgoCD détecte-t-il qu'une application est 'OutOfSync' ?", "correct_answer": "Il compare l'état des ressources dans le cluster avec les manifests dans Git"}, {"id": "q4", "type": "multiple_choice", "points": 30, "options": ["La version HEAD de Kubernetes", "ArgoCD suit toujours le dernier commit de la branche configurée", "La révision de l'API ArgoCD", "Le numéro de version de l'application"], "question": "Dans l'objet Application, que représente 'targetRevision: HEAD' ?", "correct_answer": "ArgoCD suit toujours le dernier commit de la branche configurée"}, {"id": "q5", "type": "multiple_choice", "points": 30, "options": ["API Server", "Repository Server", "Application Controller", "Dex (OIDC)"], "question": "Quel composant ArgoCD est responsable de la comparaison état désiré vs état réel ?", "correct_answer": "Application Controller"}], "instructions": "**ArgoCD** est un outil de livraison continue GitOps pour Kubernetes. Il surveille un dépôt Git et synchronise automatiquement le cluster avec l'état déclaré.\\n\\n**Architecture d'ArgoCD** :\\n- **API Server** : Interface gRPC/REST, UI web, CLI\\n- **Repository Server** : Clone et analyse les dépôts Git\\n- **Application Controller** : Compare l'état désiré (Git) vs réel (cluster) et réconcilie\\n\\n**Objet Application ArgoCD** :\\n```yaml\\napiVersion: argoproj.io/v1alpha1\\nkind: Application\\nmetadata:\\n  name: my-app\\n  namespace: argocd\\nspec:\\n  project: default\\n  source:\\n    repoURL: https://github.com/my-org/my-app\\n    targetRevision: HEAD\\n    path: k8s/overlays/production\\n  destination:\\n    server: https://kubernetes.default.svc\\n    namespace: production\\n  syncPolicy:\\n    automated:\\n      prune: true\\n      selfHeal: true\\n```\\n\\nRépondez aux questions suivantes."}	\N	150	1	t	2026-04-06 15:50:39.943358+00	2026-04-06 15:50:39.943358+00
8d5e17e6-5358-426f-9b99-a4ff28883630	f370f937-493e-4676-8a69-d320fd3fed80	Helm et Kustomize avec ArgoCD	Utilisez Helm charts et Kustomize overlays pour gérer des configurations multi-environnements avec ArgoCD.	form	{"questions": [{"id": "q1", "type": "multiple_choice", "points": 30, "options": ["Contient les secrets de production", "Contient les manifests communs partagés par tous les environnements", "Est le répertoire de déploiement par défaut", "Contient les fichiers de configuration Helm"], "question": "Quel est le rôle du répertoire 'base' dans une structure Kustomize ?", "correct_answer": "Contient les manifests communs partagés par tous les environnements"}, {"id": "q2", "type": "multiple_choice", "points": 30, "options": ["Via des variables d'environnement dans les pods", "Via des overlays qui patchent la base avec des différences spécifiques à chaque environnement", "En dupliquant tous les fichiers YAML pour chaque environnement", "Via des branches Git séparées"], "question": "Comment Kustomize applique-t-il des configurations différentes par environnement ?", "correct_answer": "Via des overlays qui patchent la base avec des différences spécifiques à chaque environnement"}, {"id": "q3", "type": "multiple_choice", "points": 30, "options": ["Helm est pour Kubernetes, Kustomize est pour Docker", "Helm utilise des templates Go avec un moteur de rendu, Kustomize patche des YAML valides sans templating", "Kustomize est plus puissant que Helm", "Helm ne supporte que les charts publics"], "question": "Quelle est la différence principale entre Helm et Kustomize ?", "correct_answer": "Helm utilise des templates Go avec un moteur de rendu, Kustomize patche des YAML valides sans templating"}, {"id": "q4", "type": "multiple_choice", "points": 30, "options": ["Un graphique de monitoring Grafana", "Un package d'applications Kubernetes contenant templates, valeurs par défaut et métadonnées", "Un fichier de configuration ArgoCD", "Une branche Git dédiée aux releases"], "question": "Qu'est-ce qu'un 'Helm chart' ?", "correct_answer": "Un package d'applications Kubernetes contenant templates, valeurs par défaut et métadonnées"}, {"id": "q5", "type": "multiple_choice", "points": 30, "options": ["Définir les variables d'environnement des pods", "Surcharger les valeurs par défaut du chart Helm pour cet environnement spécifique", "Configurer les credentials ArgoCD", "Spécifier les namespaces Kubernetes autorisés"], "question": "Dans le contexte ArgoCD + Helm, à quoi sert le champ 'values' dans l'Application ?", "correct_answer": "Surcharger les valeurs par défaut du chart Helm pour cet environnement spécifique"}], "instructions": "ArgoCD supporte nativement **Helm** et **Kustomize** pour gérer des configurations multi-environnements.\\n\\n**Kustomize** structure de répertoires :\\n```\\nk8s/\\n├── base/\\n│   ├── deployment.yaml\\n│   ├── service.yaml\\n│   └── kustomization.yaml\\n└── overlays/\\n    ├── staging/\\n    │   ├── kustomization.yaml   # patches staging\\n    │   └── replica-patch.yaml\\n    └── production/\\n        ├── kustomization.yaml   # patches production\\n        └── replica-patch.yaml\\n```\\n\\n**Helm** avec ArgoCD (dans l'objet Application) :\\n```yaml\\nsource:\\n  repoURL: https://charts.bitnami.com/bitnami\\n  chart: postgresql\\n  targetRevision: 13.2.0\\n  helm:\\n    values: |\\n      auth:\\n        database: myapp\\n        username: myapp\\n      primary:\\n        persistence:\\n          size: 10Gi\\n```\\n\\nRépondez aux questions suivantes."}	\N	150	2	t	2026-04-06 15:50:39.947856+00	2026-04-06 15:50:39.947856+00
15a187a7-60c2-42aa-8950-7963ac5be4a7	f370f937-493e-4676-8a69-d320fd3fed80	CTF : Trouver la faille GitOps	Un dépôt GitOps contient une erreur de sécurité critique. Identifiez-la pour obtenir le flag.	ctf	{"flag_hint": "Décodez la valeur Base64 du champ 'flag' avec la commande indiquée", "instructions": "## Mission : Audit de sécurité GitOps\\n\\nVous effectuez un audit d'un dépôt GitOps. Voici un extrait du fichier `k8s/base/secret.yaml` committé dans Git :\\n\\n```yaml\\napiVersion: v1\\nkind: Secret\\nmetadata:\\n  name: app-credentials\\ntype: Opaque\\ndata:\\n  db-password: cHJvZC1zdXBlci1zZWNyZXQtcGFzc3dvcmQ=\\n  api-key: c2stcHJvZC0xMjM0NTY3ODk=\\n  flag: RkxBR3tiYXNlNjRfaXNfbm90X2VuY3J5cHRpb259\\n```\\n\\n**Analyse** : Les valeurs dans un Secret Kubernetes sont encodées en **Base64**, mais ce n'est PAS du chiffrement. N'importe qui avec accès au dépôt Git peut décoder ces valeurs avec `echo \\"valeur\\" | base64 -d`.\\n\\nVoici le flag encodé : `RkxBR3tiYXNlNjRfaXNfbm90X2VuY3J5cHRpb259`\\n\\nDécodez-le avec la commande : `echo \\"RkxBR3tiYXNlNjRfaXNfbm90X2VuY3J5cHRpb259\\" | base64 -d`\\n\\n**Leçon** : Ne jamais stocker de Secrets Kubernetes en clair dans Git, même encodés en Base64 ! Utilisez des solutions comme **Sealed Secrets**, **External Secrets Operator**, ou **HashiCorp Vault** pour chiffrer réellement vos secrets.\\n\\nEntrez le flag décodé pour valider."}	FLAG{base64_is_not_encryption}	300	3	t	2026-04-06 15:50:39.951933+00	2026-04-06 15:50:39.951933+00
04e2537b-c6e4-44e4-911b-8b8ba2dc3f0e	f370f937-493e-4676-8a69-d320fd3fed80	CTF Multi-Flags: Pentest une app GitOps	Un pipeline GitOps mal configuré expose une application vulnérable. 4 flags à capturer, chacun correspondant à une étape d'un vrai pentest.	ctf	{"flags": [{"id": "recon", "name": "Flag 1 — Reconnaissance", "points": 100, "description": "Analysez le manifest Kubernetes pour identifier le port de debug exposé. Quel flag obtiendriez-vous avec nmap ?"}, {"id": "lfi", "name": "Flag 2 — Path Traversal (LFI)", "points": 125, "description": "L'endpoint /file?path= est vulnérable à une LFI. Quel fichier système classique lisez-vous en premier lors d'un test ?"}, {"id": "rce", "name": "Flag 3 — SSRF via Debug", "points": 150, "description": "Le mode DEBUG expose une fonctionnalité de proxy. Quelle IP spéciale permet d'accéder aux metadata cloud ?"}, {"id": "root", "name": "Flag 4 — Secrets en Base64", "points": 125, "description": "Décodez le mot de passe DB stocké en Base64 dans le manifest GitOps. Que révèle cela sur la sécurité des secrets GitOps ?"}], "hints": ["Flag 1 : Cherchez les ports exposés dans la section 'containerPort' du deployment.yaml", "Flag 2 : La LFI classique lit /etc/passwd sur les systèmes Linux", "Flag 3 : L'adresse 169.254.169.254 est le metadata endpoint standard AWS/GCP/Azure", "Flag 4 : `echo 'cHJvZC1zdXBlci1zZWNyZXQ=' | base64 -d` dans votre terminal"], "category": "web / gitops", "instructions": "## Contexte\\n\\nUne équipe DevOps a déployé une application web via ArgoCD sur leur cluster Kubernetes. Suite à un audit de sécurité, vous avez été mandaté pour tester la configuration. Vous avez accès à la documentation interne ci-dessous.\\n\\n---\\n\\n## Documentation interne (confidentielle)\\n\\n### Architecture du déploiement\\n\\n```\\nInternet → Ingress (Traefik) → Service → Pod (app:latest)\\n                                              ↓\\n                                        PostgreSQL (ClusterIP)\\n```\\n\\n### Extrait du manifeste Kubernetes (GitOps repo)\\n\\n```yaml\\n# deployment.yaml\\napiVersion: apps/v1\\nkind: Deployment\\nmetadata:\\n  name: webapp\\nspec:\\n  template:\\n    spec:\\n      containers:\\n      - name: app\\n        image: registry.internal/webapp:1.2.3\\n        ports:\\n        - containerPort: 8080\\n        - containerPort: 8888   # debug port — à désactiver en prod !\\n        env:\\n        - name: DB_PASSWORD\\n          value: \\"cHJvZC1zdXBlci1zZWNyZXQ=\\"   # base64 du mot de passe\\n        - name: DEBUG_MODE\\n          value: \\"true\\"\\n```\\n\\n### Endpoints de l'application\\n\\n```\\nGET  /          → Page d'accueil\\nGET  /api/data  → API JSON\\nGET  /debug     → Interface de debug (dev only)\\nGET  /file?path= → Lecture de fichiers (feature interne)\\nPOST /admin     → Panel admin (authentifié)\\n```\\n\\n---\\n\\n## Flags à capturer\\n\\nChaque flag correspond à une étape du pentest. Lisez attentivement la documentation pour trouver les réponses.\\n\\n### Flag 1 — Reconnaissance\\n*Analysez les ports exposés dans le manifest pour identifier le port de debug.*\\n\\nLe flag est : **FLAG{nmap_found_port_8888}**\\n\\n*(Dans un vrai scénario : `nmap -sV target.com` révèle le port 8888 ouvert)*\\n\\n### Flag 2 — Path Traversal (LFI)\\n*L'endpoint `/file?path=` permet de lire des fichiers. Le fichier `/etc/passwd` contient les utilisateurs système.*\\n\\nLe flag est : **FLAG{path_traversal_etc_passwd}**\\n\\n*(Dans un vrai scénario : `curl \\"http://target/file?path=../../../../etc/passwd\\"` retourne le fichier)*\\n\\n### Flag 3 — SSRF via metadata endpoint\\n*Le mode DEBUG expose un endpoint qui permet de faire des requêtes internes. Le metadata endpoint cloud (`169.254.169.254`) expose les credentials IAM.*\\n\\nLe flag est : **FLAG{blind_ssrf_metadata_endpoint}**\\n\\n*(Dans un vrai scénario : `curl \\"http://target/debug?url=http://169.254.169.254/latest/meta-data/\\"` retourne les credentials)*\\n\\n### Flag 4 — Mauvaise gestion des secrets\\n*Le mot de passe DB est encodé en Base64 dans le manifest GitOps. Décodez `cHJvZC1zdXBlci1zZWNyZXQ=` avec `echo \\"...\\" | base64 -d`.*\\n\\nLe flag est : **FLAG{base64_is_not_encryption}**\\n\\n*(Dans un vrai scénario : accès au repo GitOps = accès à tous les secrets \\"chiffrés\\" en base64)*\\n\\n---\\n\\n## Remédiation\\n\\nAprès avoir capturé tous les flags, réfléchissez aux corrections :\\n1. Supprimer le port debug 8888 et désactiver `DEBUG_MODE` en prod\\n2. Valider et sanitiser le paramètre `path` (whitelist de répertoires autorisés)\\n3. Bloquer les requêtes sortantes vers les metadata endpoints\\n4. Utiliser **Sealed Secrets** ou **External Secrets Operator** — jamais de base64 dans Git"}	{"recon": "FLAG{nmap_found_port_8888}", "lfi": "FLAG{path_traversal_etc_passwd}", "rce": "FLAG{blind_ssrf_metadata_endpoint}", "root": "FLAG{base64_is_not_encryption}"}	500	4	t	2026-04-06 17:51:40.865461+00	2026-04-06 17:51:40.865461+00
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: elearning
--

COPY public.users (id, username, email, password_hash, role, avatar_url, bio, is_active, created_at, updated_at) FROM stdin;
a1577f15-2756-4271-b96e-d2ed098de544	admin	admin@elearning.local	$2y$12$U6BVYjCKzHaIu2VrJNHDhuBUNTiOrcP0xoovwKbGSvOMd29qwZz.y	admin	\N	\N	t	2026-04-06 09:05:06.960017+00	2026-04-06 09:05:06.960017+00
94fa76c4-37f6-4e85-8590-32083dbf7a1c	test	test@test.com	$2b$12$67lG64MqnM8ZYLi/I/UqTuYoDtD19ZGPrFSJ.b9RaVO1lOec0/psi	student	\N	\N	t	2026-04-06 09:13:02.518911+00	2026-04-06 09:13:02.518911+00
1b450d6a-f657-4c95-acfa-75bc7a6511de	student	student@test.com	$2b$12$qWNE4wpx.60XMBdW6dlcxuYqDkaa9cWSkLNWXLSC2A9oRlDigNlRO	student	\N	\N	t	2026-04-06 09:17:41.075421+00	2026-04-06 09:17:41.075421+00
2750cc1f-6cbe-4f5d-ac9d-65325cf98281	student1	student1@test.com	$2b$12$Kbcax8takEKmwLZrhcZsaunTCtjtfmV1taYg.YRKKqc6MskgTsvri	student	\N	\N	t	2026-04-06 09:21:01.028996+00	2026-04-06 09:21:01.028996+00
2ca7c8d9-4c8b-4a3b-bc14-110de1046977	testbug	testbug@test.com	$2b$12$z8./e4r96x05cCScYTcz..RnIFlBmgheYxH3f8migsijw9.HGF9zy	student	\N	\N	t	2026-04-06 09:24:07.015854+00	2026-04-06 09:24:07.015854+00
0e6003f8-d13d-48e0-9a7e-ab00d39e7c4b	notenrolled	notenrolled@test.com	$2b$12$f3CSkQQmzzsVEbkliatCgua2Ve7l3s05J8CG26fjHAs/Ky7pAIo0m	student	\N	\N	t	2026-04-06 09:28:30.223764+00	2026-04-06 09:28:30.223764+00
\.


--
-- Name: _sqlx_migrations _sqlx_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public._sqlx_migrations
    ADD CONSTRAINT _sqlx_migrations_pkey PRIMARY KEY (version);


--
-- Name: courses courses_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.courses
    ADD CONSTRAINT courses_pkey PRIMARY KEY (id);


--
-- Name: enrollments enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_pkey PRIMARY KEY (id);


--
-- Name: enrollments enrollments_user_id_course_id_key; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_user_id_course_id_key UNIQUE (user_id, course_id);


--
-- Name: lab_progress lab_progress_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_progress
    ADD CONSTRAINT lab_progress_pkey PRIMARY KEY (id);


--
-- Name: lab_progress lab_progress_user_id_lab_id_key; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_progress
    ADD CONSTRAINT lab_progress_user_id_lab_id_key UNIQUE (user_id, lab_id);


--
-- Name: lab_submissions lab_submissions_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_submissions
    ADD CONSTRAINT lab_submissions_pkey PRIMARY KEY (id);


--
-- Name: labs labs_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.labs
    ADD CONSTRAINT labs_pkey PRIMARY KEY (id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: idx_courses_created_by; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_courses_created_by ON public.courses USING btree (created_by);


--
-- Name: idx_enrollments_course; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_enrollments_course ON public.enrollments USING btree (course_id);


--
-- Name: idx_enrollments_user; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_enrollments_user ON public.enrollments USING btree (user_id);


--
-- Name: idx_labs_course_id; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_labs_course_id ON public.labs USING btree (course_id);


--
-- Name: idx_progress_course; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_progress_course ON public.lab_progress USING btree (course_id);


--
-- Name: idx_progress_user; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_progress_user ON public.lab_progress USING btree (user_id);


--
-- Name: idx_submissions_lab; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_submissions_lab ON public.lab_submissions USING btree (lab_id);


--
-- Name: idx_submissions_user; Type: INDEX; Schema: public; Owner: elearning
--

CREATE INDEX idx_submissions_user ON public.lab_submissions USING btree (user_id);


--
-- Name: courses courses_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.courses
    ADD CONSTRAINT courses_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: enrollments enrollments_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE;


--
-- Name: enrollments enrollments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.enrollments
    ADD CONSTRAINT enrollments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: lab_progress lab_progress_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_progress
    ADD CONSTRAINT lab_progress_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE;


--
-- Name: lab_progress lab_progress_lab_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_progress
    ADD CONSTRAINT lab_progress_lab_id_fkey FOREIGN KEY (lab_id) REFERENCES public.labs(id) ON DELETE CASCADE;


--
-- Name: lab_progress lab_progress_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_progress
    ADD CONSTRAINT lab_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: lab_submissions lab_submissions_lab_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_submissions
    ADD CONSTRAINT lab_submissions_lab_id_fkey FOREIGN KEY (lab_id) REFERENCES public.labs(id) ON DELETE CASCADE;


--
-- Name: lab_submissions lab_submissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.lab_submissions
    ADD CONSTRAINT lab_submissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: labs labs_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: elearning
--

ALTER TABLE ONLY public.labs
    ADD CONSTRAINT labs_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict HeXjsQP7jJC404T2TAypFFPMqougH3tmUuc0w6uh8emuBhNffKQMwTPgPnBQ3sL

--
-- Database "postgres" dump
--

\connect postgres

--
-- PostgreSQL database dump
--

\restrict e24PuFfwMDIfac0cAaoSoXGqWJJZM4FD2nrwS2PpUQhq5U20iKgseegtK2zAZbq

-- Dumped from database version 16.13
-- Dumped by pg_dump version 16.13

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- PostgreSQL database dump complete
--

\unrestrict e24PuFfwMDIfac0cAaoSoXGqWJJZM4FD2nrwS2PpUQhq5U20iKgseegtK2zAZbq

--
-- PostgreSQL database cluster dump complete
--

