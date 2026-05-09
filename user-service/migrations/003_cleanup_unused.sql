-- Clean up tables no longer needed after the split into two micro-services.
-- course_settings: courses are defined exclusively via K8s CRD (Course Service).
-- git_repos: module git content is fetched per-request via FetchModuleContent.

DROP TABLE IF EXISTS course_settings;
DROP TABLE IF EXISTS git_repos;
DROP INDEX IF EXISTS idx_course_settings_source;
DROP INDEX IF EXISTS idx_git_repos_user;
