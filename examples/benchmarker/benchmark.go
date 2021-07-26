package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/creasty/defaults"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
)

func main() {
	filename := flag.String("file", "", "Input experiment file")
	flag.Parse()
	experiment := readJSON(*filename)
	writeYAMLS(experiment)
	//toYAML(experiment)
}

func readJSON(filename string) Experiment {

	// read file
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)

	}

	var result Experiment

	json.Unmarshal([]byte(data), &result)
	return result
}

func toYAML(manifest Manifest, directory string) {

	jsonBytes, err := json.Marshal(manifest)
	byteArray, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		log.Fatalf("err: %v\n", err)
		return
	}
	err = ioutil.WriteFile(directory+"/service-"+manifest.Metadata.Name+".yml", byteArray, 0644)
	if err != nil {
		log.Fatal(err)
	}
	//log.Infoln(string(y))
}

func writeYAMLS(experiment Experiment) {
	currentTime := time.Now()
	directory := fmt.Sprintf(experiment.Name + "_" + currentTime.Format("02-Feb-06_15:04:05"))
	err := os.Mkdir(directory, 0755)
	if err != nil {
		log.Fatalf("err: %v\n", err)
	}
	for _, service := range experiment.Services {
		var manifest Manifest
		if err := defaults.Set(&manifest); err != nil {
			panic(err)
		}
		manifest.Metadata.Name = service.Name
		var container Container
		if err := defaults.Set(&container); err != nil {
			panic(err)
		}
		container.Image = service.Image
		for _, env := range experiment.CommonEnv {
			if !varInEnv(env.Name, service.Env) {
				container.Env = append(container.Env, env)
			}
		}
		for _, env := range service.Env {
			container.Env = append(container.Env, env)
		}
		container.Ports = service.Ports
		manifest.Spec.Template.Spec.Containers = append(manifest.Spec.Template.Spec.Containers, container)
		toYAML(manifest, directory)
	}
}

func varInEnv(name string, envList []Env) bool {
	for _, env := range envList {
		if name == env.Name {
			return true
		}
	}
	return false
}
