package session

import "github.com/vibecode/vibecode/internal/provider"

type CacheTokens struct {
	Read  int
	Write int
}

type TokenUsage struct {
	Input     int
	Output    int
	Reasoning int
	Cache     CacheTokens
	// Total is the provider-reported total context-window size.
	// When non-zero it takes precedence over summing individual fields.
	Total int
}

type StepUsage struct {
	Tokens TokenUsage
	Cost   float64
}

func GetCost(tokens TokenUsage, modelID string) float64 {
	m := provider.LookupModel(modelID)
	input := float64(tokens.Input-tokens.Cache.Read-tokens.Cache.Write) * m.Cost.Input
	output := float64(tokens.Output) * m.Cost.Output
	cacheRead := float64(tokens.Cache.Read) * m.Cost.CacheRead
	cacheWrite := float64(tokens.Cache.Write) * m.Cost.CacheWrite
	return input + output + cacheRead + cacheWrite
}

func TotalTokens(u TokenUsage) int {
	if u.Total > 0 {
		return u.Total
	}
	return u.Input + u.Output + u.Reasoning + u.Cache.Read + u.Cache.Write
}

type SessionUsage struct {
	TotalInput     int
	TotalOutput    int
	TotalReasoning int
	TotalCacheRead int
	TotalCacheWrite int
	TotalCost      float64
}

func (s *SessionUsage) AddStep(u StepUsage) {
	s.TotalInput += u.Tokens.Input
	s.TotalOutput += u.Tokens.Output
	s.TotalReasoning += u.Tokens.Reasoning
	s.TotalCacheRead += u.Tokens.Cache.Read
	s.TotalCacheWrite += u.Tokens.Cache.Write
	s.TotalCost += u.Cost
}

func (s *SessionUsage) TotalTokens() int {
	return s.TotalInput + s.TotalOutput + s.TotalReasoning + s.TotalCacheRead + s.TotalCacheWrite
}
