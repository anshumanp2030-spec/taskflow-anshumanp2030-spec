-- Seed data for TaskFlow
-- Test user password: password123 (bcrypt cost 12)

INSERT INTO users (id, name, email, password) VALUES (
    'a0000000-0000-0000-0000-000000000001',
    'Test User',
    'test@example.com',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQyCZklCbNNNM3oUv9I/5EJAG'  -- password123
) ON CONFLICT (email) DO NOTHING;

INSERT INTO users (id, name, email, password) VALUES (
    'a0000000-0000-0000-0000-000000000002',
    'Jane Smith',
    'jane@example.com',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQyCZklCbNNNM3oUv9I/5EJAG'  -- password123
) ON CONFLICT (email) DO NOTHING;

INSERT INTO projects (id, name, description, owner_id) VALUES (
    'b0000000-0000-0000-0000-000000000001',
    'Website Redesign',
    'Full redesign of the company website for Q2 launch.',
    'a0000000-0000-0000-0000-000000000001'
) ON CONFLICT DO NOTHING;

INSERT INTO tasks (id, title, description, status, priority, project_id, assignee_id, due_date) VALUES
(
    'c0000000-0000-0000-0000-000000000001',
    'Design new homepage mockups',
    'Create Figma mockups for desktop and mobile homepage.',
    'done',
    'high',
    'b0000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0000-000000000001',
    '2026-04-10'
),
(
    'c0000000-0000-0000-0000-000000000002',
    'Implement responsive navigation',
    'Build the new navbar component in React with mobile hamburger menu.',
    'in_progress',
    'medium',
    'b0000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0000-000000000002',
    '2026-04-20'
),
(
    'c0000000-0000-0000-0000-000000000003',
    'SEO audit and fixes',
    'Run full SEO audit, fix meta tags and improve page load performance.',
    'todo',
    'low',
    'b0000000-0000-0000-0000-000000000001',
    NULL,
    '2026-05-01'
)
ON CONFLICT DO NOTHING;
