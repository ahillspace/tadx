package followers

import "github.com/ahillspace/tadx/internal/readsource"

type Input struct {
	Environment string
	Site        string
	MetricLUID  string
	Cache       bool
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
	Warnings      []string             `json:"warnings,omitempty"`
	Subscriptions []Subscription       `json:"subscriptions"`
	RequestID     string               `json:"tableau_request_id,omitempty"`
	Help          []string             `json:"help"`
	Source        *readsource.Metadata `json:"source,omitempty"`
}

type CompactSubscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
	FollowerName string `json:"follower_name"`
}

type CompactResult struct {
	Status        string                `json:"status"`
	Environment   string                `json:"environment,omitempty"`
	Site          string                `json:"site,omitempty"`
	MetricLUID    string                `json:"metric_luid"`
	Count         int                   `json:"count"`
	Warnings      []string              `json:"warnings,omitempty"`
	Subscriptions []CompactSubscription `json:"subscriptions"`
	Help          []string              `json:"help"`
	Source        *readsource.Metadata  `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	items := make([]CompactSubscription, len(o.Subscriptions))
	for i, item := range o.Subscriptions {
		items[i] = CompactSubscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName}
	}
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, MetricLUID: o.MetricLUID, Count: o.Count, Warnings: o.Warnings, Subscriptions: items, Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any { return o }
