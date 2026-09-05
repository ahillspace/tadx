package followers

import "github.com/ahillspace/tadx/internal/readsource"

type Input struct {
	Environment string
	Site        string
	MetricLUID  string
	Catalog     bool
}
type Metric struct {
	LUID      string
	RequestID string
}
type Subscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
	FollowerName string `json:"follower_name,omitempty"`
	RequestID    string `json:"-"`
}
type Output struct {
	Status        string               `json:"status"`
	Environment   string               `json:"environment,omitempty"`
	Site          string               `json:"site,omitempty"`
	MetricLUID    string               `json:"metric_luid"`
	Count         int                  `json:"count"`
	Subscriptions []Subscription       `json:"subscriptions"`
	RequestID     string               `json:"tableau_request_id,omitempty"`
	Help          []string             `json:"help"`
	Source        *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any { value := o; value.RequestID = ""; return value }
func (o Output) FullOutput() any    { return o }
