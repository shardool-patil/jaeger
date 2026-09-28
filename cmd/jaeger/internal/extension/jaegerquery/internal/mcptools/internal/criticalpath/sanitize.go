// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package criticalpath

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// removeOverflowingChildren removes or adjusts child spans that overflow their parent's time range.
// An overflowing child span is one whose time range falls outside its parent span's time range.
// The function adjusts the start time and duration of overflowing child spans
// to ensure they fit within the time range of their parent span.
func removeOverflowingChildren(spanMap map[pcommon.SpanID]CPSpan) map[pcommon.SpanID]CPSpan {
	// First, drop any spans whose parent doesn't exist (except root spans with empty ParentSpanID).
	// We iteratively drop orphans until no new orphans are found.
	for {
		droppedAny := false
		for spanID, span := range spanMap {
			if span.ParentSpanID.IsEmpty() {
				continue
			}
			if _, parentExists := spanMap[span.ParentSpanID]; !parentExists {
				dropSubtree(spanMap, spanID)
				droppedAny = true
			}
		}
		if !droppedAny {
			break
		}
	}

	// Identify all root spans (spans with empty ParentSpanID)
	for spanID, span := range spanMap {
		if span.ParentSpanID.IsEmpty() {
			sanitizeSubtree(spanMap, spanID)
		}
	}

	return spanMap
}

func sanitizeSubtree(spanMap map[pcommon.SpanID]CPSpan, parentID pcommon.SpanID) {
	parentSpan, ok := spanMap[parentID]
	if !ok {
		return
	}

	parentEndTime := parentSpan.StartTime + parentSpan.Duration
	filteredChildren := make([]pcommon.SpanID, 0, len(parentSpan.ChildSpanIDs))

	for _, childID := range parentSpan.ChildSpanIDs {
		childSpan, childExists := spanMap[childID]
		if !childExists {
			continue
		}

		childEndTime := childSpan.StartTime + childSpan.Duration

		if childSpan.StartTime >= parentSpan.StartTime {
			if childSpan.StartTime >= parentEndTime {
				// child starts after or at parent end => drop child and its subtree
				dropSubtree(spanMap, childID)
				continue
			}
			if childEndTime > parentEndTime {
				// child ends after parent => truncate duration
				childSpan.Duration = parentEndTime - childSpan.StartTime
			}
		} else {
			if childEndTime <= parentSpan.StartTime {
				// child ends before or at parent start => drop child and its subtree
				dropSubtree(spanMap, childID)
				continue
			}
			if childEndTime <= parentEndTime {
				// child starts before parent => truncate start
				childSpan.StartTime = parentSpan.StartTime
				childSpan.Duration = childEndTime - parentSpan.StartTime
			} else {
				// child starts before parent and ends after parent => truncate both
				childSpan.StartTime = parentSpan.StartTime
				childSpan.Duration = parentEndTime - parentSpan.StartTime
			}
		}

		spanMap[childID] = childSpan
		filteredChildren = append(filteredChildren, childID)

		// Recursively sanitize this child's subtree
		sanitizeSubtree(spanMap, childID)
	}

	parentSpan.ChildSpanIDs = filteredChildren
	spanMap[parentID] = parentSpan
}

func dropSubtree(spanMap map[pcommon.SpanID]CPSpan, rootID pcommon.SpanID) {
	span, ok := spanMap[rootID]
	if !ok {
		return
	}
	delete(spanMap, rootID)
	for _, childID := range span.ChildSpanIDs {
		dropSubtree(spanMap, childID)
	}
}
