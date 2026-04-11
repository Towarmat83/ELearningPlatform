-- Migration 007 — Seed: "Introduction to Linux" interactive lab
-- Creates a course + one interactive lab with clickable steps.
-- Safe to run multiple times thanks to ON CONFLICT DO NOTHING.

DO $$
DECLARE
  v_admin_id   UUID;
  v_course_id  UUID;
  v_lab_id     UUID;
BEGIN
  -- Resolve admin user
  SELECT id INTO v_admin_id FROM users WHERE role = 'admin' LIMIT 1;
  IF v_admin_id IS NULL THEN
    RAISE NOTICE 'No admin user found — skipping linux-intro seed.';
    RETURN;
  END IF;

  -- Insert course (idempotent via title uniqueness guard)
  INSERT INTO courses (title, description, category, difficulty, is_published, created_by)
  VALUES (
    'Introduction to Linux',
    'Apprends les bases de Linux en pratiquant directement dans un terminal interactif. Chaque commande est cliquable — tu n''as qu''à suivre les étapes.',
    'Linux',
    'beginner',
    TRUE,
    v_admin_id
  )
  ON CONFLICT DO NOTHING
  RETURNING id INTO v_course_id;

  -- If already seeded, find the existing course id
  IF v_course_id IS NULL THEN
    SELECT id INTO v_course_id FROM courses WHERE title = 'Introduction to Linux' LIMIT 1;
  END IF;

  -- Insert the interactive lab
  INSERT INTO labs (course_id, title, description, lab_type, content, points, order_index, is_published)
  VALUES (
    v_course_id,
    'Linux Basics: Navigation & Fichiers',
    'Explore le système de fichiers Linux, manipule des fichiers et des répertoires, et découvre les commandes essentielles.',
    'interactive',
    '{
      "docker_image": "ubuntu:22.04",
      "steps": [
        {
          "id": "step1",
          "title": "1. Où suis-je ? (pwd & ls)",
          "description": "La première chose à faire dans un terminal Linux est de savoir **où tu te trouves** et **ce qu''il y a autour de toi**.\n\n- `pwd` (**P**rint **W**orking **D**irectory) affiche le chemin du répertoire actuel.\n- `ls` liste les fichiers du répertoire courant.\n- `ls -la` affiche **tous** les fichiers (y compris les cachés) avec les permissions et les tailles.",
          "commands": [
            { "cmd": "pwd", "explanation": "Affiche le répertoire courant" },
            { "cmd": "ls", "explanation": "Liste les fichiers visibles" },
            { "cmd": "ls -la", "explanation": "Liste tous les fichiers avec permissions, tailles et dates" }
          ]
        },
        {
          "id": "step2",
          "title": "2. Se déplacer (cd)",
          "description": "`cd` (**C**hange **D**irectory) te permet de naviguer dans l''arborescence.\n\n- `cd /` va à la racine du système.\n- `cd /home` va dans le répertoire `/home`.\n- `cd ..` remonte d''un niveau.\n- `cd ~` ou simplement `cd` revient dans ton répertoire personnel.",
          "commands": [
            { "cmd": "cd /", "explanation": "Va à la racine du système de fichiers" },
            { "cmd": "ls", "explanation": "Vois ce qu''il y a à la racine" },
            { "cmd": "cd /home", "explanation": "Va dans /home" },
            { "cmd": "cd ..", "explanation": "Remonte d''un niveau" },
            { "cmd": "cd ~", "explanation": "Retourne dans ton répertoire personnel" }
          ]
        },
        {
          "id": "step3",
          "title": "3. Créer et organiser des fichiers",
          "description": "Voici les commandes essentielles pour **créer**, **lire** et **supprimer** des fichiers :\n\n- `touch nom` crée un fichier vide.\n- `mkdir dossier` crée un répertoire.\n- `echo \"texte\" > fichier` écrit du texte dans un fichier.\n- `cat fichier` affiche le contenu d''un fichier.",
          "commands": [
            { "cmd": "mkdir mon_dossier", "explanation": "Crée un répertoire nommé mon_dossier" },
            { "cmd": "cd mon_dossier", "explanation": "Entre dans ce répertoire" },
            { "cmd": "touch notes.txt", "explanation": "Crée un fichier vide nommé notes.txt" },
            { "cmd": "echo \"Bonjour Linux !\" > notes.txt", "explanation": "Écrit du texte dans notes.txt" },
            { "cmd": "cat notes.txt", "explanation": "Affiche le contenu de notes.txt" }
          ]
        },
        {
          "id": "step4",
          "title": "4. Copier, déplacer et supprimer",
          "description": "- `cp source destination` copie un fichier.\n- `mv source destination` déplace (ou renomme) un fichier.\n- `rm fichier` supprime un fichier.\n- `rm -r dossier` supprime un dossier et tout son contenu.",
          "commands": [
            { "cmd": "cp notes.txt notes_backup.txt", "explanation": "Copie notes.txt en notes_backup.txt" },
            { "cmd": "mv notes_backup.txt sauvegarde.txt", "explanation": "Renomme le fichier" },
            { "cmd": "ls -la", "explanation": "Vérifie les fichiers présents" },
            { "cmd": "rm sauvegarde.txt", "explanation": "Supprime sauvegarde.txt" },
            { "cmd": "ls", "explanation": "Confirme la suppression" }
          ]
        },
        {
          "id": "step5",
          "title": "5. Rechercher et explorer",
          "description": "- `find / -name nom` recherche un fichier par son nom.\n- `grep ''motif'' fichier` recherche un motif dans un fichier.\n- `man commande` affiche le manuel d''une commande (appuie sur `q` pour quitter).\n- `history` affiche les commandes récentes.",
          "commands": [
            { "cmd": "grep \"Linux\" notes.txt", "explanation": "Cherche le mot Linux dans notes.txt" },
            { "cmd": "find / -name \"*.txt\" 2>/dev/null", "explanation": "Trouve tous les fichiers .txt du système" },
            { "cmd": "history", "explanation": "Affiche les dernières commandes tapées" },
            { "cmd": "echo $PATH", "explanation": "Affiche les répertoires de commandes disponibles" }
          ]
        }
      ]
    }'::jsonb,
    0,
    0,
    TRUE
  )
  ON CONFLICT DO NOTHING;

END $$;
