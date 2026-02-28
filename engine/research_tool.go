package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Research engine interfaces — host applications provide concrete impls.
// ---------------------------------------------------------------------------

// ResearchDataSourceType identifies a research data source.
type ResearchDataSourceType string

const (
	ResearchSourceWeb     ResearchDataSourceType = "web"
	ResearchSourceArxiv   ResearchDataSourceType = "arxiv"
	ResearchSourceBrowser ResearchDataSourceType = "browser"
)

// ResearchRequest describes what to research.
type ResearchRequest struct {
	Query           string
	Mode            string // "simple" or "deep"
	MaxSources      int
	TimeLimit       time.Duration
	DataSources     []ResearchDataSourceType
	OutputFormat    string // "text" or "markdown"
	SuccessCriteria string
	IsDeep          bool
}

// ResearchReference is a single source reference.
type ResearchReference struct {
	Title   string
	URL     string
	Snippet string
}

// ResearchFinding is a main finding from research.
type ResearchFinding struct {
	Title       string
	Description string
}

// ResearchContradiction is a detected contradiction between sources.
type ResearchContradiction struct {
	Topic    string
	Analysis string
}

// ResearchResult is the output of a research operation.
type ResearchResult struct {
	Summary        string
	Introduction   string
	MainFindings   []ResearchFinding
	Contradictions []ResearchContradiction
	Conclusion     string
	References     []ResearchReference
	ExecutionTime  time.Duration
}

// IResearchEngine performs research queries.
type IResearchEngine interface {
	Research(ctx context.Context, request ResearchRequest) (*ResearchResult, error)
}

// ---------------------------------------------------------------------------
// ResearchTool
// ---------------------------------------------------------------------------

// ResearchTool provides research capabilities as system functions.
type ResearchTool struct {
	engine IResearchEngine
}

// NewResearchTool creates a new ResearchTool with the given engine.
// Returns nil if engine is nil.
func NewResearchTool(engine IResearchEngine) *ResearchTool {
	if engine == nil {
		return nil
	}
	return &ResearchTool{engine: engine}
}

// SimpleSearch performs a quick search.
func (t *ResearchTool) SimpleSearch(ctx context.Context, query string) (string, error) {
	if logger != nil {
		logger.Infof("Performing simple search for: %s", query)
	}

	request := ResearchRequest{
		Query:        query,
		Mode:         "simple",
		MaxSources:   5,
		TimeLimit:    30 * time.Second,
		DataSources:  []ResearchDataSourceType{ResearchSourceWeb},
		OutputFormat: "text",
	}

	result, err := t.engine.Research(ctx, request)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	var formattedResult strings.Builder
	formattedResult.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	for _, ref := range result.References {
		formattedResult.WriteString(fmt.Sprintf("- %s\n", ref.Title))
		formattedResult.WriteString(fmt.Sprintf("  %s\n", ref.URL))
		formattedResult.WriteString(fmt.Sprintf("  %s\n\n", ref.Snippet))
	}

	return formattedResult.String(), nil
}

// DeepResearch performs comprehensive research using multiple sources.
func (t *ResearchTool) DeepResearch(ctx context.Context, query string, successCriteria string) (string, error) {
	if logger != nil {
		logger.Infof("Performing deep research for: %s", query)
	}

	request := ResearchRequest{
		Query:           query,
		Mode:            "deep",
		MaxSources:      10,
		TimeLimit:       15 * time.Minute,
		DataSources:     []ResearchDataSourceType{}, // empty means use all available
		OutputFormat:    "markdown",
		SuccessCriteria: successCriteria,
		IsDeep:          true,
	}

	result, err := t.engine.Research(ctx, request)
	if err != nil {
		return "", fmt.Errorf("research failed: %w", err)
	}

	var output strings.Builder

	output.WriteString(fmt.Sprintf("# Research Report: %s\n\n", query))

	output.WriteString("## Executive Summary\n\n")
	output.WriteString(result.Summary)
	output.WriteString("\n\n")

	output.WriteString("## Introduction\n\n")
	output.WriteString(result.Introduction)
	output.WriteString("\n\n")

	output.WriteString("## Main Findings\n\n")
	for i, finding := range result.MainFindings {
		output.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, finding.Title))
		output.WriteString(finding.Description)
		output.WriteString("\n\n")
	}

	if len(result.Contradictions) > 0 {
		output.WriteString("## Contradictions and Conflicts\n\n")
		for i, contradiction := range result.Contradictions {
			output.WriteString(fmt.Sprintf("### Contradiction %d: %s\n\n", i+1, contradiction.Topic))
			output.WriteString(contradiction.Analysis)
			output.WriteString("\n\n")
		}
	}

	output.WriteString("## Conclusion\n\n")
	output.WriteString(result.Conclusion)
	output.WriteString("\n\n")

	output.WriteString("## References\n\n")
	for i, ref := range result.References {
		output.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, ref.Title, ref.URL))
	}

	output.WriteString(fmt.Sprintf("\n\n*Research completed in %s*", result.ExecutionTime.String()))

	return output.String(), nil
}
