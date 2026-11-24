package value

import "time"

// TDSScope uses the canonical server and identities supplied by PAT sign-in.
type TDSScope struct {
	Server     string `json:"server"`
	SiteLUID   string `json:"site_luid"`
	CallerLUID string `json:"caller_luid"`
}

// TDSProjection retains exactly what to read and a digest of the expected
// presence and value. Receipt storage never needs raw descriptions.
type TDSProjection struct {
	ElementType string `json:"element_type"`
	Name        string `json:"name"`
	Property    string `json:"property"`
	Digest      string `json:"digest"`
}

// TDSCheckoutContext records when the client observed source identity and
// received Tableau's checkout acknowledgement. Later source changes do not
// update these historical observations.
type TDSCheckoutContext struct {
	SourceName        string `json:"source_name,omitempty"`
	SourceProjectLUID string `json:"source_project_luid,omitempty"`
	SourceObservedAt  string `json:"source_observed_at"`
	AcknowledgedAt    string `json:"acknowledged_at,omitempty"`
}

type TDSAttempt struct {
	Checkout           *TDSCheckoutContext `json:"checkout_receipt,omitempty"`
	ID                 string              `json:"attempt_id"`
	Scope              TDSScope            `json:"scope"`
	Operation          string              `json:"operation"`
	Status             string              `json:"submit_status"`
	SourceLUID         string              `json:"source_luid,omitempty"`
	WorkCopyLUID       string              `json:"work_copy_luid,omitempty"`
	PublishedLUID      string              `json:"published_luid,omitempty"`
	UnexpectedIdentity string              `json:"additional_provider_reported_luid,omitempty"`
	ReportedLUID       string              `json:"provider_reported_luid,omitempty"`
	RequestID          string              `json:"tableau_request_id,omitempty"`
	SourceRevision     string              `json:"source_revision,omitempty"`
	IntentDigest       string              `json:"intent_digest,omitempty"`
	DestinationDigest  string              `json:"destination_digest,omitempty"`
	CompareDescription bool                `json:"compare_description,omitempty"`
	CompleteCollection bool                `json:"complete_field_description_collection,omitempty"`
	Projections        []TDSProjection     `json:"projections,omitempty"`
	Verification       string              `json:"verification,omitempty"`
	SupersededBy       string              `json:"acknowledged_by_attempt,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
}
