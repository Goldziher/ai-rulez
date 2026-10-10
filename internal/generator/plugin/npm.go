package plugin

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// composeNPMPackages merges runtime resource metadata before path de-duplication;
// otherwise a combined Pi/OpenCode bundle silently loses one runtime's manifest.
func composeNPMPackages(outputs []config.OutputFile, baseDir string) ([]config.OutputFile, error) {
	manifestPath := filepath.Join(baseDir, "package.json")
	count := 0
	for _, out := range outputs {
		if out.Path == manifestPath {
			count++
		}
	}
	if count < 2 {
		return outputs, nil
	}
	var merged map[string]any
	var result []config.OutputFile
	for _, out := range outputs {
		if out.Path != manifestPath {
			result = append(result, out)
			continue
		}
		var pkg map[string]any
		if err := json.Unmarshal(out.RawContent, &pkg); err != nil {
			return nil, oops.Wrapf(err, "parse runtime npm package")
		}
		if merged == nil {
			merged = pkg
			continue
		}
		if err := mergeNPMPackage(merged, pkg); err != nil {
			return nil, err
		}
	}
	if merged == nil {
		return outputs, nil
	}
	out, err := jsonOutput(manifestPath, merged)
	if err != nil {
		return nil, err
	}
	return append(result, out), nil
}

func mergeNPMPackage(merged, pkg map[string]any) error {
	for key, value := range pkg {
		previous, exists := merged[key]
		if !exists {
			merged[key] = value
			continue
		}
		if key == "files" || key == "keywords" {
			previousList, previousOK := previous.([]any)
			valueList, valueOK := value.([]any)
			if !previousOK || !valueOK {
				merged[key] = value
				continue
			}
			values := append([]any{}, previousList...)
			for _, item := range valueList {
				if !slices.Contains(values, item) {
					values = append(values, item)
				}
			}
			slices.SortFunc(values, func(a, b any) int {
				as, aok := a.(string)
				bs, bok := b.(string)
				if !aok || !bok {
					return 0
				}
				return strings.Compare(as, bs)
			})
			merged[key] = values
			continue
		}
		if !reflect.DeepEqual(previous, value) {
			return oops.With("field", key).Errorf("runtime npm manifests disagree")
		}
	}
	return nil
}
