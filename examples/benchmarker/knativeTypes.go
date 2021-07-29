package main

type Manifest struct {
	APIVersion string    `json:"apiVersion" default:"serving.knative.dev/v1"`
	Kind       string    `json:"kind" default:"Service"`
	Metadata   Metadata  `json:"metadata"`
	Spec       OuterSpec `json:"spec"`
}
type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace" default:"default"`
}
type Env struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Port struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
}
type Container struct {
	Image           string `json:"image"`
	ImagePullPolicy string `json:"imagePullPolicy"  default:"Always"`
	Env             []Env  `json:"env"`
	Ports           []Port `json:"ports"`
}
type Spec struct {
	Containers []Container `json:"containers"`
	ContainerConcurrency string `json:"containerConcurrency"`
}

type Annotations struct {
	MinScale string `json:"autoscaling.knative.dev/minScale"`
}

type ScaleMetaData struct {
	Annotations Annotations `json:"annotations"`
}

type Template struct {
	Spec Spec `json:"spec"`
	ScaleMetaData ScaleMetaData `json:"metadata"`
}
type OuterSpec struct {
	Template Template `json:"template"`
}
