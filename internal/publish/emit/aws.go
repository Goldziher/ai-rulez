package emit

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/samber/oops"
)

// Limits of CreateRegistryRecord
// (https://docs.aws.amazon.com/agent-registry-control/latest/APIReference/API_CreateRegistryRecord.html, checked 2026-10-06).
const (
	awsNameMax        = 255
	awsDescriptionMax = 4096
	awsSkillSchema    = "0.1.0"
)

var (
	awsNamePattern    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-./]*$`)
	awsVersionPattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
	awsTagPattern     = regexp.MustCompile(`^[a-zA-Z0-9\s._:/=+@-]*$`)
)

type awsSkillMd struct {
	Data string `json:"data"`
}

type awsAdditional struct {
	SkillMd awsSkillMd `json:"skillMd"`
}

type awsSkillsDef struct {
	AdditionalData    awsAdditional `json:"additionalData"`
	Data              string        `json:"data"`
	DataSchemaVersion string        `json:"dataSchemaVersion"`
}

type awsCustom struct {
	Data string `json:"data"`
}

type awsDescriptors struct {
	AgentSkillsDefinition *awsSkillsDef `json:"agentSkillsDefinition,omitempty"`
	Custom                *awsCustom    `json:"custom,omitempty"`
}

// awsRecord is the request body of CreateRegistryRecord (registryId is a URI
// parameter, supplied when the record is created).
type awsRecord struct {
	Description   string            `json:"description,omitempty"`
	Descriptors   awsDescriptors    `json:"descriptors"`
	DisplayName   string            `json:"displayName,omitempty"`
	Name          string            `json:"name"`
	RecordType    string            `json:"recordType"`
	RecordVersion string            `json:"recordVersion,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

type awsPackage struct {
	RegistryType string `json:"registryType"`
	Identifier   string `json:"identifier"`
	Version      string `json:"version,omitempty"`
}

type awsRepository struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

type awsSkillDefinition struct {
	Repository *awsRepository `json:"repository,omitempty"`
	Packages   []awsPackage   `json:"packages,omitempty"`
}

type awsIndexEntry struct {
	File       string `json:"file"`
	Name       string `json:"name"`
	RecordType string `json:"record_type"`
}

type awsIndex struct {
	Records []awsIndexEntry `json:"records"`
}

// awsRegistry writes one AWS Agent Registry record per skill (recordType SKILL,
// descriptor agentSkillsDefinition with the SKILL.md as additionalData.skillMd)
// and one CUSTOM record per plugin. The files are request bodies for
// CreateRegistryRecord; nothing is sent.
type awsRegistry struct{}

func init() { register(awsRegistry{}) }

func (awsRegistry) Name() string   { return "aws-agent-registry" }
func (awsRegistry) Status() string { return StatusExperimental }

func (awsRegistry) Emit(in Input) ([]File, []Finding, error) {
	var (
		files []File
		idx   awsIndex
	)
	add := func(rec awsRecord) error {
		if err := checkAWSRecord(rec); err != nil {
			return err
		}
		data, err := jsonBytes(rec)
		if err != nil {
			return err
		}
		file := "records/" + strings.ReplaceAll(rec.Name, "/", "_") + ".json"
		files = append(files, File{Path: file, Data: data})
		idx.Records = append(idx.Records, awsIndexEntry{File: file, Name: rec.Name, RecordType: rec.RecordType})
		return nil
	}
	repo := awsRepositoryOf(in.Repo)
	for i := range in.Plugins {
		p := &in.Plugins[i]
		tags := map[string]string{"ai-rulez-plugin": p.Name}
		if in.Commit != "" {
			tags["commit"] = in.Commit
		}
		summary, err := json.Marshal(map[string]any{
			keyName: p.Name, keyVersion: p.Version, keyDescription: p.Description, "runtimes": nonNil(p.Runtimes),
			"bundle": p.BundleFile, "bundle_digest": p.BundleDigest, "lock_tree": in.LockTree,
		})
		if err != nil {
			return nil, nil, oops.Wrapf(err, "encode the plugin record")
		}
		if err := add(awsRecord{
			Description: p.Description, DisplayName: p.Name, Name: p.Name, RecordType: "CUSTOM",
			RecordVersion: p.Version, Tags: tags, Descriptors: awsDescriptors{Custom: &awsCustom{Data: string(summary)}},
		}); err != nil {
			return nil, nil, err
		}
		for _, s := range Skills(p.Files) {
			def, err := json.Marshal(awsSkillDefinition{Repository: repo})
			if err != nil {
				return nil, nil, oops.Wrapf(err, "encode the skill definition")
			}
			if err := add(awsRecord{
				Description: s.Description, DisplayName: s.Name, Name: p.Name + "/" + s.Name, RecordType: "SKILL",
				RecordVersion: p.Version, Tags: tags,
				Descriptors: awsDescriptors{AgentSkillsDefinition: &awsSkillsDef{
					AdditionalData:    awsAdditional{SkillMd: awsSkillMd{Data: s.Body}},
					Data:              string(def),
					DataSchemaVersion: awsSkillSchema,
				}},
			}); err != nil {
				return nil, nil, err
			}
		}
	}
	index, err := jsonBytes(idx)
	if err != nil {
		return nil, nil, err
	}
	files = append(files, File{Path: "index.json", Data: index})
	out, err := finish(files)
	return out, nil, err
}

func awsRepositoryOf(repo string) *awsRepository {
	if repo == "" {
		return nil
	}
	switch strings.Count(repo, "/") {
	case 1:
		return &awsRepository{URL: "https://github.com/" + repo, Source: "github"}
	case 2:
		return &awsRepository{URL: "https://" + repo, Source: strings.SplitN(repo, ".", 2)[0]}
	}
	return nil
}

// checkAWSRecord enforces the documented request constraints so an invalid
// record fails here rather than at the service.
func checkAWSRecord(r awsRecord) error {
	switch {
	case r.Name == "" || len(r.Name) > awsNameMax || !awsNamePattern.MatchString(r.Name):
		return oops.Errorf("aws-agent-registry: %q is not a valid record name (letters, digits, '_', '-', '.', '/'; at most %d)", r.Name, awsNameMax)
	case r.RecordVersion != "" && (len(r.RecordVersion) > awsNameMax || !awsVersionPattern.MatchString(r.RecordVersion)):
		return oops.Hint("the registry accepts letters, digits, '.' and '-' in a record version").
			Errorf("aws-agent-registry: version %q of %s is not a valid record version", r.RecordVersion, r.Name)
	case len(r.Description) > awsDescriptionMax:
		return oops.Errorf("aws-agent-registry: the description of %s exceeds %d characters", r.Name, awsDescriptionMax)
	case len(r.DisplayName) > awsNameMax:
		return oops.Errorf("aws-agent-registry: the display name of %s exceeds %d characters", r.Name, awsNameMax)
	}
	for k, v := range r.Tags {
		if k == "" || len(k) > 128 || len(v) > 256 || !awsTagPattern.MatchString(k) || !awsTagPattern.MatchString(v) {
			return oops.Errorf("aws-agent-registry: tag %q of %s is not valid", k, r.Name)
		}
	}
	return nil
}
