package unfollow

type Input struct {
	Environment      string
	Site             string
	SubscriptionLUID string
	MetricLUID       string
	UserLUID         string
	GroupLUID        string
}
type Subscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid,omitempty"`
	FollowerType string `json:"follower_type,omitempty"`
	FollowerLUID string `json:"follower_luid,omitempty"`
	FollowerName string `json:"follower_name,omitempty"`
}
type Plan struct {
	Mode             string `json:"mode"`
	Operation        string `json:"operation"`
	Environment      string `json:"environment,omitempty"`
	Site             string `json:"site,omitempty"`
	SubscriptionLUID string `json:"subscription_luid"`
	MetricLUID       string `json:"metric_luid,omitempty"`
	FollowerType     string `json:"follower_type,omitempty"`
	FollowerLUID     string `json:"follower_luid,omitempty"`
}
type Result struct {
	Status           string `json:"status"`
	SubscriptionLUID string `json:"subscription_luid"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
