package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

// Test file with ignored error returns in Go

func readFileUnsafe(filename string) string {
	// Violation: Ignoring error from ReadFile
	content, _ := ioutil.ReadFile(filename)
	return string(content)
}

func makeHTTPRequest(url string) {
	// Violation: Not checking error from http.Get
	resp, _ := http.Get(url)
	defer resp.Body.Close()

	// Violation: Ignoring error from ReadAll
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func writeFileUnsafe(filename string, data []byte) {
	// Violation: Ignoring error from WriteFile
	_ = ioutil.WriteFile(filename, data, 0644)
}

func parseIntUnsafe(s string) int {
	// Violation: Ignoring error from Atoi
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

func openFileUnsafe(filename string) *os.File {
	// Violation: Not handling error from Open
	file, _ := os.Open(filename)
	return file
}

// Violation: Function without error return when it should have one
func riskyOperation() {
	file, err := os.Open("test.txt")
	if err != nil {
		// Violation: Using panic instead of returning error
		panic(err)
	}
	defer file.Close()
}

func main() {
	// Violation: Multiple ignored errors
	resp, _ := http.Get("http://example.com")
	body, _ := ioutil.ReadAll(resp.Body)
	_ = ioutil.WriteFile("output.txt", body, 0644)

	// Violation: Printf without checking return
	fmt.Printf("Done")
}
