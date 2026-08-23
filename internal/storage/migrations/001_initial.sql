PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('tenant_admin','operator','reviewer','data_steward','worker')),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, email)
);

CREATE INDEX IF NOT EXISTS users_tenant_role_idx ON users(tenant_id, role, active);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS auth_sessions_user_idx ON auth_sessions(tenant_id, user_id, expires_at);

CREATE TABLE IF NOT EXISTS facilities (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS capture_rigs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    facility_id TEXT NOT NULL REFERENCES facilities(id),
    name TEXT NOT NULL,
    capabilities INTEGER NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, facility_id, name)
);

CREATE INDEX IF NOT EXISTS capture_rigs_available_idx ON capture_rigs(tenant_id, facility_id, active);

CREATE TABLE IF NOT EXISTS scenarios (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    environment TEXT NOT NULL,
    required_capabilities INTEGER NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS capture_sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    facility_id TEXT NOT NULL REFERENCES facilities(id),
    scenario_id TEXT NOT NULL REFERENCES scenarios(id),
    rig_id TEXT NOT NULL REFERENCES capture_rigs(id),
    operator_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('planned','ready','recording','processing','validated','rejected','canceled','archived')),
    revision INTEGER NOT NULL DEFAULT 1,
    consent_ref TEXT NOT NULL,
    started_at TEXT,
    submitted_at TEXT,
    validated_at TEXT,
    canceled_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS capture_sessions_tenant_status_idx ON capture_sessions(tenant_id, status, updated_at);

CREATE TABLE IF NOT EXISTS rig_leases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    rig_id TEXT NOT NULL REFERENCES capture_rigs(id),
    capture_id TEXT NOT NULL REFERENCES capture_sessions(id),
    owner TEXT NOT NULL,
    token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    released_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, rig_id, capture_id)
);

CREATE INDEX IF NOT EXISTS rig_leases_one_live_idx
ON rig_leases(tenant_id, rig_id) WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS rig_leases_expiry_idx ON rig_leases(tenant_id, expires_at, released_at);

CREATE TABLE IF NOT EXISTS stream_manifests (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    capture_id TEXT NOT NULL REFERENCES capture_sessions(id),
    kind TEXT NOT NULL CHECK (kind IN ('pose','force','trajectory','first_person_video','third_person_video')),
    status TEXT NOT NULL CHECK (status IN ('open','sealed','aligned','invalid')),
    segment_count INTEGER NOT NULL DEFAULT 0,
    first_nanos INTEGER NOT NULL DEFAULT 0,
    last_nanos INTEGER NOT NULL DEFAULT 0,
    digest TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, capture_id, kind)
);

CREATE TABLE IF NOT EXISTS stream_segments (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    manifest_id TEXT NOT NULL REFERENCES stream_manifests(id),
    sequence INTEGER NOT NULL,
    start_nanos INTEGER NOT NULL,
    end_nanos INTEGER NOT NULL,
    object_uri TEXT NOT NULL,
    checksum TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, manifest_id, sequence),
    UNIQUE (tenant_id, manifest_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS stream_segments_order_idx ON stream_segments(tenant_id, manifest_id, sequence);

CREATE TABLE IF NOT EXISTS annotation_batches (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    capture_id TEXT NOT NULL REFERENCES capture_sessions(id),
    status TEXT NOT NULL CHECK (status IN ('open','claimed','submitted','accepted','rework','canceled')),
    owner TEXT NOT NULL DEFAULT '',
    lease_token TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT,
    submitted_at TEXT,
    reviewed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS annotation_batches_claim_idx ON annotation_batches(tenant_id, status, lease_expires_at, created_at);

CREATE TABLE IF NOT EXISTS annotation_items (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    batch_id TEXT NOT NULL REFERENCES annotation_batches(id),
    segment_id TEXT NOT NULL REFERENCES stream_segments(id),
    label TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    complete INTEGER NOT NULL DEFAULT 0 CHECK (complete IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, batch_id, segment_id)
);

CREATE TABLE IF NOT EXISTS quality_reviews (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    reviewer_id TEXT NOT NULL REFERENCES users(id),
    outcome TEXT NOT NULL CHECK (outcome IN ('approved','rejected','rework')),
    reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, object_type, object_id, version)
);

CREATE INDEX IF NOT EXISTS quality_reviews_object_idx ON quality_reviews(tenant_id, object_type, object_id, created_at);

CREATE TABLE IF NOT EXISTS dataset_drafts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft','frozen','approved','published','revoked','archived')),
    revision INTEGER NOT NULL DEFAULT 1,
    digest TEXT NOT NULL DEFAULT '',
    item_count INTEGER NOT NULL DEFAULT 0,
    frozen_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, name, revision)
);

CREATE TABLE IF NOT EXISTS dataset_items (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES dataset_drafts(id),
    capture_id TEXT NOT NULL REFERENCES capture_sessions(id),
    revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, dataset_id, capture_id)
);

CREATE INDEX IF NOT EXISTS dataset_items_dataset_idx ON dataset_items(tenant_id, dataset_id, created_at);

CREATE TABLE IF NOT EXISTS dataset_releases (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES dataset_drafts(id),
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('approved','published','revoked','archived')),
    published_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, dataset_id, revision)
);

CREATE TABLE IF NOT EXISTS release_items (
    release_id TEXT NOT NULL REFERENCES dataset_releases(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (release_id, dataset_item_id)
);

CREATE TABLE IF NOT EXISTS training_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    release_id TEXT NOT NULL REFERENCES dataset_releases(id),
    status TEXT NOT NULL CHECK (status IN ('queued','running','retrying','succeeded','failed','canceled')),
    owner TEXT NOT NULL DEFAULT '',
    lease_token TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL,
    checkpoint TEXT NOT NULL DEFAULT '',
    output_uri TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS training_jobs_claim_idx ON training_jobs(tenant_id, status, next_attempt_at, lease_expires_at);

CREATE TABLE IF NOT EXISTS job_attempts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    job_id TEXT NOT NULL REFERENCES training_jobs(id),
    attempt INTEGER NOT NULL,
    worker_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    outcome TEXT NOT NULL DEFAULT 'running',
    error_text TEXT NOT NULL DEFAULT '',
    UNIQUE (tenant_id, job_id, attempt)
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    topic TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','delivering','delivered','dead')),
    owner TEXT NOT NULL DEFAULT '',
    lease_token TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    next_attempt_at TEXT NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS outbox_claim_idx ON outbox_events(tenant_id, status, next_attempt_at, lease_expires_at);

CREATE TABLE IF NOT EXISTS idempotency_records (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    response BLOB NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    UNIQUE (tenant_id, method, path, key)
);

CREATE INDEX IF NOT EXISTS idempotency_expiry_idx ON idempotency_records(expires_at);

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    request_id TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_object_idx ON audit_events(tenant_id, object_type, object_id, created_at);
CREATE INDEX IF NOT EXISTS audit_request_idx ON audit_events(tenant_id, request_id, created_at);
