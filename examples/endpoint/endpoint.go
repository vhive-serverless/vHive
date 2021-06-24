package endpoint

type Endpoint struct {
	// Hostname of the starting point of a workflow/pipeline. Its
	// difference from a URL is not containing a port number.
	Hostname string            `json:"hostname"`
	// True if the workflow uses Knative Eventing and thus its
	// duration/latency is to be measured using TimeseriesDB.
	// TimeseriesDB must have been deployed and configured correctly!
	Eventing bool              `json:"eventing"`
	// Only relevant if Eventing is true, Matchers is a mapping of
	// CloudEvent attribute names and values that are sought in
	// completion events to exist.
	Matchers map[string]string `json:"matchers"`
}
