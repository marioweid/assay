package api

import (
	"sort"

	"github.com/marioweid/assay/assayd/internal/domain"
)

type spanTreeNode struct {
	response *spanResponse
	parent   int
	state    uint8
}

const (
	spanUnvisited uint8 = iota
	spanVisiting
	spanDone
)

// spanTree presents every stored span exactly once as a forest whose roots are
// orphans, self-parented spans, and the earliest member of each parent cycle.
// Duplicate OTel span IDs resolve children to the earliest stored row.
func spanTree(spans []domain.Span) []*spanResponse {
	nodes := resolveSpanParents(spans)
	breakParentCycles(nodes)
	return attachSpanChildren(nodes)
}

func resolveSpanParents(spans []domain.Span) []spanTreeNode {
	if len(spans) == 0 {
		return nil
	}
	ordered := make([]domain.Span, len(spans))
	copy(ordered, spans)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].StartTime.Equal(ordered[j].StartTime) {
			return ordered[i].StartTime.Before(ordered[j].StartTime)
		}
		return ordered[i].ID < ordered[j].ID
	})

	nodes := make([]spanTreeNode, len(ordered))
	byOTelID := make(map[string][]int, len(ordered))
	for index, span := range ordered {
		response := spanOutput(span)
		nodes[index] = spanTreeNode{response: response, parent: -1}
		byOTelID[response.OTelSpanID] = append(byOTelID[response.OTelSpanID], index)
	}
	assignSpanParents(nodes, byOTelID)
	return nodes
}

func assignSpanParents(nodes []spanTreeNode, byOTelID map[string][]int) {
	canonical := make(map[string]int, len(byOTelID))
	for id, indices := range byOTelID {
		canonical[id] = indices[0]
	}
	for index, node := range nodes {
		if node.response.ParentSpanID == nil {
			continue
		}
		if parent, found := canonical[*node.response.ParentSpanID]; found && parent != index {
			nodes[index].parent = parent
		}
	}
}

func breakParentCycles(nodes []spanTreeNode) {
	for start := range nodes {
		if nodes[start].state != spanUnvisited {
			continue
		}
		path := make([]int, 0, len(nodes))
		current := start
		for {
			if current < 0 || nodes[current].state == spanDone {
				markSpanPathDone(nodes, path)
				break
			}
			if nodes[current].state == spanVisiting {
				cycleStart := pathIndex(path, current)
				nodes[earliestCycleIndex(path[cycleStart:])].parent = -1
				markSpanPathDone(nodes, path)
				break
			}
			nodes[current].state = spanVisiting
			path = append(path, current)
			current = nodes[current].parent
		}
	}
}

func pathIndex(path []int, target int) int {
	for index, value := range path {
		if value == target {
			return index
		}
	}
	return len(path)
}

func earliestCycleIndex(cycle []int) int {
	earliest := cycle[0]
	for _, index := range cycle[1:] {
		if index < earliest {
			earliest = index
		}
	}
	return earliest
}

func markSpanPathDone(nodes []spanTreeNode, path []int) {
	for _, index := range path {
		nodes[index].state = spanDone
	}
}

func attachSpanChildren(nodes []spanTreeNode) []*spanResponse {
	roots := make([]*spanResponse, 0, len(nodes))
	for index := range nodes {
		if nodes[index].parent < 0 {
			roots = append(roots, nodes[index].response)
			continue
		}
		parent := nodes[nodes[index].parent]
		parent.response.Children = append(parent.response.Children, nodes[index].response)
	}
	return roots
}
