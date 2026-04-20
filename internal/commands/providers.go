package commands

import (
	"fmt"
	"strings"

	"github.com/vibecode/vibecode/internal/openrouter"
	"github.com/vibecode/vibecode/internal/provider"
)

func (r *Registry) registerProviders() {
	r.Register(Command{
		Name:        "providers",
		Aliases:     []string{"p"},
		Description: "List available AI providers",
		Type:        TypeLocal,
		Handler:     r.providersHandler,
	})
}

func (r *Registry) providersHandler(args string) Result {
	client := openrouter.NewClient()
	data, err := openrouter.GlobalCache.FetchOrGet(client)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to fetch providers: %v", err)}
	}
	provider.BuildRegistryFromOpenRouter(data)

	var b strings.Builder
	b.WriteString("Available providers:\n\n")

	for _, pm := range data {
		meta, known := provider.ProviderMetaMap[pm.Provider.Slug]
		name := pm.Provider.Name
		if known {
			name = meta.Name
		}
		b.WriteString(fmt.Sprintf("  %-20s %s (%d models)\n", pm.Provider.Slug, name, len(pm.Models)))
	}

	b.WriteString(fmt.Sprintf("\nTotal: %d providers with models\n", len(data)))
	return Result{Output: b.String()}
}
