package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
)

func main() {
	filename := flag.String("file", "", "Input experiment file")
	flag.Parse()
	splitName := strings.Split(*filename, ".")
	extension := splitName[len(splitName)-1]

	var experiment Experiment
	if strings.EqualFold(extension, "json") {
		experiment = readJSON(*filename)
	} else if strings.EqualFold(extension, "yaml") || strings.EqualFold(extension, "yml") {
		experiment = readYAML(*filename)
	}
	experimentCopy := experiment
	for _, value := range experiment.Tunable.Values {
		if varInEnv(experiment.Tunable.Name, experiment.CommonEnv) {
			log.Fatalf("can't have tunable as common env variable")
		} else {
			var env Env
			env.Name = experiment.Tunable.Name
			env.Value = value
			experiment.CommonEnv = append(experiment.CommonEnv, env)
		}
		writeYAMLS(experiment, experiment.Tunable.Name, value)
		experiment = experimentCopy
	}
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

func readYAML(filename string) Experiment {

	// read file
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)

	}

	var result Experiment

	err = yaml.Unmarshal(data, &result)
	if err != nil {
		log.Fatal(err)
	}
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

func writeYAMLS(experiment Experiment, tag, value string) {
	currentTime := time.Now()
	directory := fmt.Sprintf("%s_[%s-%s]_%s", experiment.Name, tag, value, currentTime.Format("02-Feb-06_15:04:05"))
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
		if service.Scale.MinScale == "tunable" {
			manifest.Spec.Template.ScaleMetaData.Annotations.MinScale = value
		}
		if service.ContainerConcurrency == "tunable"{
			manifest.Spec.Template.Spec.ContainerConcurrency = value
		}
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
