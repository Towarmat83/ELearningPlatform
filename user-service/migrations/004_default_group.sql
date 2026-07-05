-- Create the "everyone" default group (all users belong to it automatically)
INSERT INTO groups (name, source, description)
VALUES ('everyone', 'local', 'Default group — all users are members automatically')
ON CONFLICT (name) DO NOTHING;

-- Backfill all existing users into this group
INSERT INTO user_groups (userId, groupId)
SELECT u.id, g.id
FROM users u
CROSS JOIN groups g
WHERE g.name = 'everyone'
ON CONFLICT DO NOTHING;
