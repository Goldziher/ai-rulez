package crud

import (
	"path/filepath"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/agentssource"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// readsAgentsMD reports whether the context items of the shared root include
// the project's AGENTS.md: the "agents" source, root scope, not the machine-local tree.
func (op *OperatorImpl) readsAgentsMD(domain, ftype string) bool {
	return op.layout.Agents && !op.local && domain == "" && ftype == ContentTypeContext
}

// agentsMDNames names the AGENTS.md items as the loader does (see agentssource.ItemNames).
func (op *OperatorImpl) agentsMDNames() []string {
	return agentssource.ItemNames(op.baseDir, op.layout.AgentsMDSplit, op.contextFileNames())
}

// contextFileNames are the names of the files under .agents/context.
func (op *OperatorImpl) contextFileNames() map[string]bool {
	taken := map[string]bool{}
	files, err := op.listFilesInDirectory("", ContentTypeContext)
	if err != nil {
		return taken
	}
	for i := range files {
		taken[files[i].Name] = true
	}
	return taken
}

// isAgentsMDItem reports whether name is an item of AGENTS.md in this scope.
func (op *OperatorImpl) isAgentsMDItem(domain, ftype, name string) bool {
	return op.readsAgentsMD(domain, ftype) && slices.Contains(op.agentsMDNames(), name)
}

// withAgentsMDItems puts the AGENTS.md items before the listed context files,
// the order the loader gives them.
func (op *OperatorImpl) withAgentsMDItems(domain, ftype string, listed []FileInfo) []FileInfo {
	if !op.readsAgentsMD(domain, ftype) {
		return listed
	}
	path := filepath.Join(op.baseDir, config.AgentsMDFilename)
	var out []FileInfo
	for _, name := range op.agentsMDNames() {
		out = append(out, FileInfo{Name: name, Path: path, Type: ftype})
	}
	return append(out, listed...)
}

// removeAgentsMDItem deletes one section of AGENTS.md (see agentssource.RemoveItem).
func (op *OperatorImpl) removeAgentsMDItem(name string) error {
	return agentssource.RemoveItem(op.baseDir, name, op.layout.AgentsMDSplit, op.contextFileNames())
}
