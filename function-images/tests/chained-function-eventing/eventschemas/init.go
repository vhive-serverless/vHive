package eventschemas

type GreetingEventBody struct {
	Name string `json:"name,string"`
}
