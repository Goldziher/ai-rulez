package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/progress"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
)

const rolesDeliveryConfig = lockProjectConfig + `
[[roles]]
name = "dev"
domains = ["backend"]
[roles.delivery]
"deploy*" = "served"

[[roles]]
name = "ops"
extends = "dev"
[roles.delivery]
"deploy-prod" = "static"

[[roles]]
name = "plain"
domains = ["backend"]
`

func rolesDeliveryProject(t *testing.T) string {
	t.Helper()
	root := lockProject(t, "")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rolesDeliveryConfig)
	for _, id := range []string{"migrate", "deploy", "deploy-prod"} {
		writeFile(t, filepath.Join(root, ".ai-rulez", "domains", "backend", "skills", id, "SKILL.md"),
			"---\nname: "+id+"\ndescription: Use when you need "+id+".\n---\nBody of "+id+".\n")
	}
	t.Cleanup(func() { rolesFormat, tokensRole, tokensByRole, tokensJSON, catalogFormat = "", "", false, false, "" })
	return root
}

func TestRolesReportDelivery(t *testing.T) {
	rolesDeliveryProject(t)
	rolesFormat = formatJSON

	var out bytes.Buffer
	require.NoError(t, runRolesList(&out))
	validateAgainst(t, "../../schema/roles-manifest.schema.json", out.Bytes())
	var manifest roles.Manifest
	require.NoError(t, json.Unmarshal(out.Bytes(), &manifest))
	byName := map[string]roles.Role{}
	for _, r := range manifest.Roles {
		byName[r.Name] = r
	}
	assert.Equal(t, map[string]string{"deploy*": "served", "deploy-prod": "static"}, byName["ops"].Delivery, "inherited and own entries")

	deliveryOf := func(role string) map[string]string {
		got := map[string]string{}
		for _, it := range byName[role].Items {
			if it.Kind == "skill" {
				got[it.ID] = it.Delivery
			}
		}
		return got
	}
	assert.Equal(t, "served", deliveryOf("dev")["deploy"], "the glob also matches the root skill of the same name")
	assert.Equal(t, "static", deliveryOf("dev")["migrate"])
	assert.Equal(t, "served", deliveryOf("dev")["deploy-prod"])
	assert.Equal(t, "static", deliveryOf("ops")["deploy-prod"], "the child's more specific entry wins")
	assert.Equal(t, "served", deliveryOf("ops")["deploy"])
	assert.Equal(t, "static", deliveryOf("plain")["deploy"])
	assert.Equal(t, 3, byName["dev"].Totals.Served, "root deploy, backend deploy and backend deploy-prod")
	assert.Equal(t, 2, byName["ops"].Totals.Served)
	assert.Zero(t, byName["plain"].Totals.Served)
	assert.Positive(t, byName["dev"].Totals.ServedTokens)
}

func TestCatalogReportsDeliveryPerRole(t *testing.T) {
	rolesDeliveryProject(t)
	catalogFormat = formatJSON
	var out bytes.Buffer
	require.NoError(t, runCatalog(&out))
	validateAgainst(t, "../../schema/catalog.v1.schema.json", out.Bytes())

	var doc catalogDoc
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	var deploy *catalogItem
	for i := range doc.Items {
		if doc.Items[i].Kind == "skill" && doc.Items[i].ID == "deploy" && doc.Items[i].Domain == "backend" {
			deploy = &doc.Items[i]
		}
	}
	require.NotNil(t, deploy)
	assert.Equal(t, "static", deploy.Delivery, "the item's own delivery")
	assert.Equal(t, map[string]string{"dev": "served", "ops": "served"}, deploy.RoleDelivery, "roles that differ from it; plain matches, so it is not listed")
	for _, r := range doc.Roles {
		if r.Name == "dev" {
			assert.Equal(t, 3, r.Totals.Served)
			assert.Equal(t, map[string]string{"deploy*": "served"}, r.Delivery)
		}
	}
}

func TestTokensRoleCountsServedSkillsAsNotListed(t *testing.T) {
	rolesDeliveryProject(t)
	progress.SetQuiet(true)
	tokensJSON = true

	report := func(role string) (served []string, listing int) {
		tokensRole = role
		var out bytes.Buffer
		_, err := runTokens(&out)
		require.NoError(t, err)
		var r struct {
			ServedSkills []string `json:"served_skills"`
			Listing      int      `json:"headline_listing"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &r))
		return r.ServedSkills, r.Listing
	}
	servedDev, listingDev := report("dev")
	servedPlain, listingPlain := report("plain")
	assert.Equal(t, []string{"backend/deploy", "backend/deploy-prod", "deploy"}, servedDev)
	assert.Empty(t, servedPlain)
	assert.Less(t, listingDev, listingPlain, "three served skills leave the listing; one small stub skill replaces them")
}
