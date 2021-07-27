package main

type Experiment struct {
	Name      string    `json:"name"`
	Services  []Service `json:"services"`
	CommonEnv []Env     `json:"commonEnv"`
	Tunable   struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	} `json:"tunable"`
}

type Service struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Env   []Env  `json:"env"`
	Ports []Port `json:"ports"`
}
