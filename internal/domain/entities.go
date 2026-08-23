package domain

import "time"

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"`
}

type Role string

const (
	RoleTenantAdmin Role = "tenant_admin"
	RoleOperator    Role = "operator"
	RoleReviewer    Role = "reviewer"
	RoleDataSteward Role = "data_steward"
	RoleWorker      Role = "worker"
)

func (r Role) Valid() bool {
	switch r {
	case RoleTenantAdmin, RoleOperator, RoleReviewer, RoleDataSteward, RoleWorker:
		return true
	default:
		return false
	}
}

type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Version      int64     `json:"version"`
}

type AuthSession struct {
	ID        string
	TenantID  string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	Version   int64
}

func (s AuthSession) UsableAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

type Facility struct {
	ID        string
	TenantID  string
	Name      string
	Timezone  string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

type RigCapability uint64

const (
	CapabilityPose RigCapability = 1 << iota
	CapabilityForce
	CapabilityTrajectory
	CapabilityFirstPersonVideo
	CapabilityThirdPersonVideo
	CapabilityOpticalTracking
	CapabilityInertialTracking
)

type CaptureRig struct {
	ID           string
	TenantID     string
	FacilityID   string
	Name         string
	Capabilities RigCapability
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int64
}

func (r CaptureRig) Supports(required RigCapability) bool {
	return required != 0 && r.Capabilities&required == required
}

type RigLease struct {
	ID         string
	TenantID   string
	RigID      string
	CaptureID  string
	Owner      string
	Token      string
	ExpiresAt  time.Time
	ReleasedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int64
}

func (l RigLease) ActiveAt(now time.Time) bool {
	return l.ReleasedAt == nil && now.Before(l.ExpiresAt)
}

type Scenario struct {
	ID                   string
	TenantID             string
	Name                 string
	Environment          string
	RequiredCapabilities RigCapability
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Version              int64
}

type CaptureSession struct {
	ID          string
	TenantID    string
	FacilityID  string
	ScenarioID  string
	RigID       string
	OperatorID  string
	Status      CaptureStatus
	Revision    int64
	ConsentRef  string
	StartedAt   *time.Time
	SubmittedAt *time.Time
	ValidatedAt *time.Time
	CanceledAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int64
}

type StreamKind string

const (
	StreamPose             StreamKind = "pose"
	StreamForce            StreamKind = "force"
	StreamTrajectory       StreamKind = "trajectory"
	StreamFirstPersonVideo StreamKind = "first_person_video"
	StreamThirdPersonVideo StreamKind = "third_person_video"
)

func (k StreamKind) Valid() bool {
	switch k {
	case StreamPose, StreamForce, StreamTrajectory, StreamFirstPersonVideo, StreamThirdPersonVideo:
		return true
	default:
		return false
	}
}

type StreamManifest struct {
	ID           string
	TenantID     string
	CaptureID    string
	Kind         StreamKind
	Status       ManifestStatus
	SegmentCount int
	FirstNanos   int64
	LastNanos    int64
	Digest       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int64
}

type StreamSegment struct {
	ID             string
	TenantID       string
	ManifestID     string
	Sequence       int
	StartNanos     int64
	EndNanos       int64
	ObjectURI      string
	Checksum       string
	IdempotencyKey string
	CreatedAt      time.Time
}

type AnnotationBatch struct {
	ID             string
	TenantID       string
	CaptureID      string
	Status         AnnotationStatus
	Owner          string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	SubmittedAt    *time.Time
	ReviewedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

type AnnotationItem struct {
	ID        string
	TenantID  string
	BatchID   string
	SegmentID string
	Label     string
	Payload   string
	Complete  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

type QualityReview struct {
	ID         string
	TenantID   string
	ObjectType string
	ObjectID   string
	ReviewerID string
	Outcome    string
	Reason     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int64
}

type DatasetDraft struct {
	ID        string
	TenantID  string
	Name      string
	Status    DatasetStatus
	Revision  int64
	Digest    string
	ItemCount int
	CreatedAt time.Time
	UpdatedAt time.Time
	FrozenAt  *time.Time
	Version   int64
}

type DatasetItem struct {
	ID        string
	TenantID  string
	DatasetID string
	CaptureID string
	Revision  int64
	CreatedAt time.Time
}

type DatasetRelease struct {
	ID          string
	TenantID    string
	DatasetID   string
	Revision    int64
	Digest      string
	Status      DatasetStatus
	PublishedAt *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int64
}

type TrainingJob struct {
	ID             string
	TenantID       string
	ReleaseID      string
	Status         JobStatus
	Owner          string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	AttemptCount   int
	MaxAttempts    int
	Checkpoint     string
	OutputURI      string
	LastError      string
	NextAttemptAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

type JobAttempt struct {
	ID         string
	TenantID   string
	JobID      string
	Attempt    int
	WorkerID   string
	StartedAt  time.Time
	FinishedAt *time.Time
	Outcome    string
	ErrorText  string
}

type OutboxEvent struct {
	ID             string
	TenantID       string
	Topic          string
	AggregateType  string
	AggregateID    string
	Payload        string
	Status         OutboxStatus
	Owner          string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	AttemptCount   int
	MaxAttempts    int
	NextAttemptAt  time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int64
}

type AuditEvent struct {
	ID         string
	TenantID   string
	ActorID    string
	Action     string
	ObjectType string
	ObjectID   string
	Outcome    string
	RequestID  string
	Detail     string
	CreatedAt  time.Time
}

type IdempotencyRecord struct {
	ID          string
	TenantID    string
	Method      string
	Path        string
	Key         string
	Fingerprint string
	StatusCode  int
	Response    []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
