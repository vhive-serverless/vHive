// MIT License
//
// Copyright (c) 2021 Amory Hoste and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"bufio"
	"flag"
	"net/url"
	"os"
	"os/exec"
	"path"

	log "github.com/sirupsen/logrus"
)

func main() {
	sourceRepo := flag.String("source", "docker://docker.io", "URL of the source registry")
	destRepo := flag.String("destination", "docker://docker-registry.registry.svc.cluster.local:5000", "URL of the destination registry")
	imageFile := flag.String("imageFile", "images.txt", "File with image URLs")

	flag.Parse()

	log.Infof("Reading the images from the file: %s", *imageFile)

	images, err := readLines(*imageFile)
	if err != nil {
		log.Fatal("Failed to read the image file:", err)
	}

	log.Infof("Pulling images from %s to %s", *sourceRepo, *destRepo)
	pullImages(images, *sourceRepo, *destRepo)

	log.Infoln("Images pulled to local registry")
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func pullImages(images []string, sourceRepo string, destRepo string) {
	for _, image := range images {
		pullImage(image, sourceRepo, destRepo)
	}
}

func buildURL(image string, repo string) (string, error) {
	u, err := url.Parse(repo)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, image)
	return u.String(), nil
}

func pullImage(image string, sourceRepo string, destRepo string) {
	log.Infof("Pulling %s", image)
	sourceURL, err := buildURL(image, sourceRepo)
	if err != nil {
		log.Fatalf("failed to build url: %v", err)
	}

	destURL, err := buildURL(image, destRepo)
	if err != nil {
		log.Fatalf("failed to build url: %v", err)
	}

	cmd := exec.Command(
		"skopeo",
		"copy",
		sourceURL,
		destURL,
		"--dest-tls-verify=false",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to pull image %s: %v\n%s\n", image, err, stdoutStderr)
	}
}
