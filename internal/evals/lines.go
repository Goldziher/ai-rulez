package evals

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// caseLines holds the 1-based line of each case in a YAML file.
type caseLines struct{ cases []int }

// yamlCaseLines finds the line of every entry of the top-level cases list.
func yamlCaseLines(data []byte) caseLines {
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) == 0 {
		return caseLines{}
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return caseLines{}
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "cases" && top.Content[i+1].Kind == yaml.SequenceNode {
			var out []int
			for _, item := range top.Content[i+1].Content {
				out = append(out, item.Line)
			}
			return caseLines{cases: out}
		}
	}
	return caseLines{}
}

var yamlLinePattern = regexp.MustCompile(`line (\d+)`)

func yamlErrLine(err error) int {
	if match := yamlLinePattern.FindStringSubmatch(err.Error()); match != nil {
		n, _ := strconv.Atoi(match[1]) //nolint:errcheck // the pattern only matches digits
		return n
	}
	return 1
}

// yamlErrMessage drops the "yaml: " prefix.
func yamlErrMessage(err error) string {
	return strings.TrimPrefix(err.Error(), "yaml: ")
}

// jsonLine returns the line of a JSON syntax error, or 1.
func jsonLine(data []byte, err error) int {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return 1 + bytes.Count(data[:min(int(syntax.Offset), len(data))], []byte("\n"))
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return 1 + bytes.Count(data, []byte("\n"))
	}
	return 1
}
