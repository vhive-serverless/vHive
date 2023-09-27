// MIT License
//
// Copyright (c) 2023 Siyang Shao and vHive community
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
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Print("Hello GPU received a request.")
	target := os.Getenv("TARGET")
	if target == "" {
		target = "GPU"
	}
	fmt.Fprintf(w, "Hello %s!\n", target)

	command := `nvidia-smi`
	cmd := exec.Command("/bin/bash", "-c", command)
	output, err := cmd.Output()

	if err != nil {
		fmt.Fprintf(w, "Nvidia Error")
		return
	}
	fmt.Fprint(w, string(output))
}

func main() {
	log.Print("Hello GPU sample started.")

	http.HandleFunc("/", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
