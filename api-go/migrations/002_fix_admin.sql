-- Fix admin password hash (bcrypt cost 12, password: Admin@1234)
UPDATE users
SET password_hash = '$2y$12$U6BVYjCKzHaIu2VrJNHDhuBUNTiOrcP0xoovwKbGSvOMd29qwZz.y'
WHERE username = 'admin' AND email = 'admin@elearning.local';
