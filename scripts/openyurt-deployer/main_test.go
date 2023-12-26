// MIT License
//
// Copyright (c) 2023 Jason Chua, Ruiqi Lai and vHive team
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
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestReadAndUnMarshall(t *testing.T) {
	// Create a temporary file with JSON content
	tempFile, err := ioutil.TempFile("", "test.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Define your sample JSON content
	jsonContent := `{
		"master": "username@masterip",
		"workers": {
			"cloud": [
				"username@cloudip"
			],
			"edge": [
				"username@edgeip"
			]
		}
	}`

	// Write the JSON content to the temporary file
	err = ioutil.WriteFile(tempFile.Name(), []byte(jsonContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test the readAndUnMarshall function with the temporary file
	result, err := readAndUnMarshall(tempFile.Name())
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	t.Logf("Unmarshall: %s", result)
	// Define the expected result based on your JSON structure
	expected := NodesInfo{
		// Initialize the fields based on your JSON structure
		Master: "username@masterip",
		Workers: Workers{
			Cloud: []string{"username@cloudip"},
			Edge:  []string{"username@edgeip"},
		},
	}

	t.Logf("expected res: %s", expected)
	// Compare the result with the expected value
	if !jsonEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// jsonEqual checks if two JSON objects are equal.
func jsonEqual(a, b interface{}) bool {
	ajson, err := json.Marshal(a)
	if err != nil {
		return false
	}

	bjson, err := json.Marshal(b)
	if err != nil {
		return false
	}

	return string(ajson) == string(bjson)
}

func TestParseNodeInfo(t *testing.T) {

	// Mock NodesInfo
	mockNodesInfo := NodesInfo{
		Master: "runner@127.0.0.1",
		// Workers: Workers{
		// 	Cloud: []string{"cloud@host"},
		// 	Edge:  []string{"edge@host"},
		// },
	}

	// Define a table of criteria
	criteriaTable := map[string]string{
		"Golang":     "1.19.10",
		"containerd": "1.6.18",
		"runc":       "1.1.4",
		"CNI":        "1.2.0",
	}

	// Capture standard output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Error creating pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Redirect standard output to the pipe
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	// Call the function to be tested
	nodeList := parseNodeInfo(mockNodesInfo)
	t.Logf("nodeList: %v", nodeList)

	// Capture stdout
	w.Close()
	var capturedOutput strings.Builder
	_, err = io.Copy(&capturedOutput, r)
	if err != nil {
		t.Fatalf("Error reading from pipe: %v", err)
	}
	t.Logf("captop: %v", capturedOutput.String())

	// Line split stdout
	lines := strings.Split(capturedOutput.String(), "\n")
	for _, line := range lines {
		// Example: Check for keywords and versions
		if strings.Contains(line, " Golang(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["Golang"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["Golang"], "Golang")
			}
		} else if strings.Contains(line, "containerd(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["containerd"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["containerd"], "containerd")
			}
		} else if strings.Contains(line, "runc(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["runc"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["runc"], "runc")
			}
		} else if strings.Contains(line, "CNI plugins(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["CNI"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["CNI"], "CNI")
			}
		}
	}
}

func TestDeployNodes(t *testing.T) {
	// Create a temporary file with JSON content
	tempFile, err := ioutil.TempFile("", "test.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Define your sample JSON content
	jsonContent := `{
			"master": "runner@127.0.0.1",
			"workers": {
				"cloud": [
				],
				"edge": [
				]
			}
		}`

	// Write the JSON content to the temporary file
	err = ioutil.WriteFile(tempFile.Name(), []byte(jsonContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	criteriaTable := map[string]string{
		"Golang":     "1.19.10",
		"containerd": "1.6.18",
		"runc":       "1.1.4",
		"CNI":        "1.2.0",
		"Kubernetes": "1.25.9",
	}

	// Capture standard output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Error creating pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Redirect standard output to the pipe
	oldStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	// // Call the function to be tested
	deployNodes(tempFile.Name())

	// Capture stdout
	w.Close()
	var capturedOutput strings.Builder
	_, err = io.Copy(&capturedOutput, r)
	if err != nil {
		t.Fatalf("Error reading from pipe: %v", err)
	}
	t.Logf("captop: %v", capturedOutput.String())

	// Line split stdout
	lines := strings.Split(capturedOutput.String(), "\n")
	for _, line := range lines {
		// Example: Check for keywords and versions
		if strings.Contains(line, " Golang(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["Golang"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["Golang"], "Golang")
			}
		} else if strings.Contains(line, "containerd(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["containerd"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["containerd"], "containerd")
			}
		} else if strings.Contains(line, "runc(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["runc"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["runc"], "runc")
			}
		} else if strings.Contains(line, "CNI plugins(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["CNI"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["CNI"], "CNI")
			}
		} else if strings.Contains(line, "Kubernetes(version") {
			t.Logf("line: %s", line)
			if !strings.Contains(line, criteriaTable["Kubernetes"]) {
				t.Logf("failing: %s", line)
				t.Errorf("Expected version %s not found in output for keyword %s", criteriaTable["Kubernetes"], "Kubernetes")
			}
		}
	}
}
