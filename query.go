package kgalias

import (
	"math"
	"sort"
)

// confidenceThreshold 是邻居查询的置信度下限。
const confidenceThreshold = 0.5

// confidence 计算边的置信度：支持文档数 / sqrt(两端文档频次之积)。
func confidence(support int, freqA, freqB int) float64 {
	if freqA == 0 || freqB == 0 {
		return 0
	}
	return float64(support) / math.Sqrt(float64(freqA)*float64(freqB))
}

// Node 返回节点信息：文档频次，以及置信度 >= 0.5 的邻边（按实体编号排序）。
func (g *Graph) Node(entity int64) NodeInfo {
	info := NodeInfo{Entity: entity, DocFreq: g.docFreq[entity]}
	freqA := g.docFreq[entity]
	if freqA == 0 {
		return info
	}
	for k := range g.adj[entity] {
		docs := g.edgeSupport[k]
		var other int64
		if k.a == entity {
			other = k.b
		} else if k.b == entity {
			other = k.a
		} else {
			continue
		}
		support := len(docs)
		conf := confidence(support, freqA, g.docFreq[other])
		if conf >= confidenceThreshold {
			info.Edges = append(info.Edges, EdgeInfo{
				EntityA:    k.a,
				EntityB:    k.b,
				Support:    support,
				Confidence: conf,
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

// SupportDocs 返回两个实体间完整的有序支持文档编号集合，不受置信度阈值影响。
func (g *Graph) SupportDocs(a, b int64) []int64 {
	if a == b {
		return nil
	}
	docs := g.edgeSupport[makeEdgeKey(a, b)]
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
