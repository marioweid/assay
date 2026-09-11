package api

import (
	"testing"
	"time"

	"github.com/marioweid/assay/assayd/internal/domain"
)

func TestSpanTreePresentsEverySpanExactlyOnce(t *testing.T) {
	for _, fixture := range spanTreeFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			roots := spanTree(fixture.spans)
			collected := collectSpanResponses(roots)
			if len(collected) != len(fixture.spans) {
				t.Fatalf(
					"collected %d spans, want %d; roots = %#v",
					len(collected), len(fixture.spans), spanTreeRootNames(roots),
				)
			}
			seen := make(map[string]bool, len(fixture.spans))
			for _, collectedSpan := range collected {
				otelID := hexSpanID(collectedSpan.OTelSpanID)
				if seen[otelID] {
					t.Fatalf("span %s appears more than once", otelID)
				}
				seen[otelID] = true
			}
		})
	}
}

func TestSpanTreeBreaksParentCyclesDeterministically(t *testing.T) {
	first, second := duplicateNameSpan("a", 1, "earliest"), duplicateNameSpan("a", 2, "later")
	parent := hexBytes("a")
	child := domain.Span{
		OTelSpanID: hexBytes("c"), ParentSpanID: &parent, Name: "c",
		StartTime: time.Unix(3, 0),
	}
	roots := spanTree([]domain.Span{first, child, second})

	byID := map[string]*spanResponse{}
	var walk func([]*spanResponse)
	walk = func(nodes []*spanResponse) {
		for _, node := range nodes {
			byID[node.Name] = node
			walk(node.Children)
		}
	}
	walk(roots)

	if byID["earliest"] == nil || byID["later"] == nil || byID["c"] == nil {
		t.Fatalf("duplicate parent spans missing: %#v", byID)
	}
	if len(byID["later"].Children) != 0 {
		t.Fatalf("duplicate child attached to later span: %#v", byID["later"])
	}
	if len(byID["earliest"].Children) != 1 || byID["earliest"].Children[0].Name != "c" {
		t.Fatalf("child did not attach to earliest duplicate: %#v", byID["earliest"])
	}
}

func collectSpanResponses(roots []*spanResponse) []*spanResponse {
	collected := make([]*spanResponse, 0)
	seen := make(map[*spanResponse]bool)
	stack := append([]*spanResponse{}, roots...)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil || seen[node] {
			continue
		}
		seen[node] = true
		collected = append(collected, node)
		stack = append(stack, node.Children...)
	}
	return collected
}

func spanTreeRootNames(roots []*spanResponse) []string {
	names := make([]string, 0, len(roots))
	for _, root := range roots {
		names = append(names, root.Name)
	}
	return names
}

func TestSpanTreeIsStableUnderShuffledRows(t *testing.T) {
	fixture := spanTreeFixtures()[len(spanTreeFixtures())-1]
	spans := make([]domain.Span, len(fixture.spans))
	copy(spans, fixture.spans)
	ordered := spanTreeStructure(spanTree(spans))
	for left, right := 0, len(spans)-1; left < right; left, right = left+1, right-1 {
		spans[left], spans[right] = spans[right], spans[left]
	}
	shuffled := spanTreeStructure(spanTree(spans))
	if ordered != shuffled {
		t.Fatalf("shuffled input changed tree from %q to %q", ordered, shuffled)
	}
}

func spanTreeStructure(roots []*spanResponse) string {
	var structure string
	var walk func(nodes []*spanResponse)
	walk = func(nodes []*spanResponse) {
		for _, node := range nodes {
			structure += node.Name + "("
			walk(node.Children)
			structure += ")"
		}
	}
	walk(roots)
	return structure
}

func spanTreeFixtures() []struct {
	name  string
	spans []domain.Span
} {
	return []struct {
		name  string
		spans []domain.Span
	}{
		{name: "empty"},
		{name: "root and child", spans: []domain.Span{
			namedSpan("root", nil, 1, time.Unix(1, 0)),
			namedSpan("child", hexPointer("root"), 2, time.Unix(2, 0)),
		}},
		{name: "orphan becomes root", spans: []domain.Span{
			namedSpan("root", nil, 1, time.Unix(1, 0)),
			namedSpan("orphan", hexPointer("missing"), 2, time.Unix(2, 0)),
		}},
		{name: "self parent becomes root", spans: []domain.Span{
			namedSpan("self", hexPointer("self"), 1, time.Unix(1, 0)),
		}},
		{name: "two node cycle", spans: []domain.Span{
			namedSpan("a", hexPointer("b"), 1, time.Unix(1, 0)),
			namedSpan("b", hexPointer("a"), 2, time.Unix(2, 0)),
		}},
		{name: "three node cycle with leaf", spans: []domain.Span{
			namedSpan("a", hexPointer("c"), 1, time.Unix(1, 0)),
			namedSpan("b", hexPointer("a"), 2, time.Unix(2, 0)),
			namedSpan("c", hexPointer("b"), 3, time.Unix(3, 0)),
			namedSpan("leaf", hexPointer("c"), 4, time.Unix(4, 0)),
		}},
		{name: "equal start times", spans: []domain.Span{
			namedSpan("a", nil, 2, time.Unix(5, 0)),
			namedSpan("b", hexPointer("a"), 1, time.Unix(5, 0)),
			namedSpan("c", hexPointer("a"), 3, time.Unix(5, 0)),
		}},
		{name: "deep chain", spans: chainSpans(8)},
	}
}

func namedSpan(name string, parent *[8]byte, rowID int64, at time.Time) domain.Span {
	span := domain.Span{
		ID: rowID, OTelSpanID: hexBytes(name), ParentSpanID: parent,
		Name: name, StartTime: at, EndTime: at.Add(time.Second),
	}
	return span
}

func duplicateNameSpan(name string, rowID int64, label string) domain.Span {
	span := namedSpan(name, nil, rowID, time.Unix(rowID, 0))
	span.Name = label
	return span
}

func chainSpans(depth int) []domain.Span {
	spans := make([]domain.Span, 0, depth)
	for index := 0; index < depth; index++ {
		var parent *[8]byte
		if index > 0 {
			parent = hexPointer(string(rune('a' + index - 1)))
		}
		spans = append(spans, namedSpan(
			string(rune('a'+index)), parent, int64(index+1), time.Unix(int64(index+1), 0),
		))
	}
	return spans
}

func hexPointer(value string) *[8]byte {
	decoded := hexBytes(value)
	return &decoded
}

func hexBytes(value string) [8]byte {
	var result [8]byte
	for index := range min(len(value), 8) {
		result[index] = value[index]
	}
	return result
}

func hexSpanID(value string) string {
	return value
}
