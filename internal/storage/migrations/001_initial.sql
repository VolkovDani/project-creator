CREATE TABLE projects (
 id TEXT PRIMARY KEY,
 workspace TEXT NOT NULL CHECK(workspace <> ''),
 name TEXT NOT NULL, goal TEXT NOT NULL, description TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(workspace,id)
);
CREATE INDEX projects_workspace_order ON projects(workspace,created_at,id);
CREATE TABLE requirements (
 id TEXT PRIMARY KEY,
 workspace TEXT NOT NULL,
 project_id TEXT NOT NULL,
 text TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 FOREIGN KEY(workspace,project_id) REFERENCES projects(workspace,id)
);
CREATE INDEX requirements_project_order ON requirements(workspace,project_id,created_at,id);
