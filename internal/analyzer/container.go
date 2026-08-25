package analyzer

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// ServiceDefinition maps the JSON representation of an individual service.
//
// It is a framework-agnostic shape: any container (Symfony, Laravel, Laminas,
// …) can emit `{ "class": "...", "shared": bool }` entries so Igor knows which
// classes are real shared services and which are transient (per-request /
// per-resolution) value objects that must never be flagged.
type ServiceDefinition struct {
	Class  string `json:"class"`
	Shared bool   `json:"shared"`
}

// ContainerDump is the root structure of the JSON payload accepted by the
// `--container-dump` flag, mirroring `{ "services": [ ... ] }`.
type ContainerDump struct {
	Services []ServiceDefinition `json:"services"`
	Aliases  AliasesMap          `json:"aliases"`
}

// NonSharedServiceMap acts as an O(1) lookup table for transient classes,
// keyed by fully-qualified class name (without a leading backslash).
type NonSharedServiceMap map[string]bool

// AliasesMap maps interface FQCNs to their concrete implementation FQCNs,
// used to refine the reachability call graph.
type AliasesMap map[string]string

// LoadContainerDump reads and decodes the JSON file, returning a populated map
// of non-shared (transient) classes and a map of interface aliases.
//
// An empty filePath yields empty (non-nil) maps and no error, so callers can
// invoke it unconditionally. Class names are normalized by trimming any leading
// namespace separator so lookups match the visitor's resolved FQCN.
func LoadContainerDump(filePath string) (NonSharedServiceMap, AliasesMap, error) {
	nonSharedMap := make(NonSharedServiceMap)
	aliases := make(AliasesMap)

	if filePath == "" {
		return nonSharedMap, aliases, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, err
	}

	var dump ContainerDump
	if err := json.Unmarshal(bytes, &dump); err != nil {
		return nil, nil, err
	}

	for _, service := range dump.Services {
		class := strings.TrimPrefix(service.Class, "\\")
		if class == "" {
			continue
		}
		if !service.Shared {
			nonSharedMap[class] = true
		}
	}

	for interfaceName, target := range dump.Aliases {
		interfaceName = strings.TrimPrefix(interfaceName, "\\")
		target = strings.TrimPrefix(target, "\\")
		if interfaceName == "" || target == "" {
			continue
		}
		aliases[interfaceName] = target
	}

	return nonSharedMap, aliases, nil
}
