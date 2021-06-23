package endpoint

type Endpoint struct {
	Hostname string            `json:"hostname"`
	Eventing bool              `json:"eventing"`
	Matchers map[string]string `json:"matchers"`
}
