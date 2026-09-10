package follow

import "context"

type Input struct {
	Environment string
	Site        string
	MetricLUID  string
	UserLUID    string
	GroupLUID   string
}
type CreateRequest struct {
	MetricLUID   string
	FollowerType string
	FollowerLUID string
}
type CreateResult struct {
	Status           string `json:"status"`
	SubscriptionLUID string `json:"subscription_luid,omitempty"`
	RequestID        string `json:"tableau_request_id,omitempty"`
}

// Metric, User, and Group are the minimal authoritative identities required
// before a Pulse subscription can be previewed or created.
type Metric struct{ LUID string }
type User struct{ LUID string }
type Group struct{ LUID string }

// Resolver performs exact, type-specific live identity checks.
// Implementations must not substitute or enumerate principals.
type Resolver interface {
	ResolveMetric(context.Context, string) (Metric, error)
	ResolveUser(context.Context, string) (User, error)
	ResolveGroup(context.Context, string) (Group, error)
}
type Plan struct {
	Mode         string `json:"mode"`
	Operation    string `json:"operation"`
	Environment  string `json:"environment,omitempty"`
	Site         string `json:"site,omitempty"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
}
type Output struct {
	Plan   Plan          `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}

func (o Output) CompactOutput() any {
	value := o
	if value.Result != nil {
		copy := *value.Result
		copy.RequestID = ""
		value.Result = &copy
	}
	return value
}
func (o Output) FullOutput() any { return o }
