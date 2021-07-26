package main

type Experiment struct {
	Name      string    `json:"name"`
	Services  []Service `json:"services"`
	CommonEnv []Env     `json:"commonEnv"`
}

type Service struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Env   []Env  `json:"env"`
	Ports []Port `json:"ports"`
}
