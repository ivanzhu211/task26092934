package kgalias

import (
	"math"
	"sort"
)

// reference 是一个暴力参考实现：每次查询都基于当前文档与当前绑定从头构图。
// 测试用它逐次核对增量实现的结果。
type reference struct {
	docs    map[int64][][]string
	binding map[string]int64
}

func newReference() *reference {
	return &reference{docs: make(map[int64][][]string), binding: make(map[string]int64)}
}

func (r *reference) rebuild() (map[int64]int, map[edgeKey]map[int64]struct{}) {
	freq := make(map[int64]int)
	support := make(map[edgeKey]map[int64]struct{})
	for id, sentences := range r.docs {
		entities, edges := computeProjection(sentences, r.binding)
		for e := range entities {
			freq[e]++
		}
		for k := range edges {
			set := support[k]
			if set == nil {
				set = make(map[int64]struct{})
				support[k] = set
			}
			set[id] = struct{}{}
		}
	}
	return freq, support
}

func (r *reference) node(entity int64) NodeInfo {
	freq, support := r.rebuild()
	info := NodeInfo{Entity: entity, DocFreq: freq[entity]}
	for k, docs := range support {
		if k.a != entity && k.b != entity {
			continue
		}
		conf := float64(len(docs)) / math.Sqrt(float64(freq[k.a])*float64(freq[k.b]))
		if conf >= confidenceThreshold {
			info.Edges = append(info.Edges, EdgeInfo{
				EntityA: k.a, EntityB: k.b,
				Support: len(docs), Confidence: conf,
			})
		}
	}
	sort.Slice(info.Edges, func(i, j int) bool {
		a, b := info.Edges[i], info.Edges[j]
		if a.EntityA != b.EntityA {
			return a.EntityA < b.EntityA
		}
		return a.EntityB < b.EntityB
	})
	return info
}

func (r *reference) supportDocs(a, b int64) []int64 {
	if a == b {
		return nil
	}
	_, support := r.rebuild()
	docs := support[makeEdgeKey(a, b)]
	if len(docs) == 0 {
		return nil
	}
	out := make([]int64, 0, len(docs))
	for id := range docs {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
