package kgalias

import (
	"math"
	"sort"
)

// minConfidence 是邻居查询的置信度阈值。
const minConfidence = 0.5

// DocumentFrequency 返回实体的文档频次；实体不在当前图谱中时返回 0。
func (g *Graph) DocumentFrequency(e EntityID) int {
	return len(g.entityDocs[e])
}

// Neighbors 返回节点频次，以及置信度至少 0.5 的邻边和各边支持数。
// 置信度等于支持文档数除以两端节点文档频次乘积的平方根。
// 结果按邻居实体编号升序排列。
func (g *Graph) Neighbors(e EntityID) NeighborResult {
	res := NeighborResult{DocFreq: len(g.entityDocs[e])}
	if res.DocFreq == 0 {
		return res
	}
	denom := math.Sqrt(float64(res.DocFreq))
	// 遍历该实体的支持文档，收集候选邻居，避免扫描全图。
	candidates := make(map[EntityID]int)
	for docID := range g.entityDocs[e] {
		doc := g.docs[docID]
		for p := range doc.contrib.pairs {
			var other EntityID
			switch {
			case p.A == e:
				other = p.B
			case p.B == e:
				other = p.A
			default:
				continue
			}
			if _, ok := candidates[other]; !ok {
				candidates[other] = len(g.edgeDocs[p])
			}
		}
	}
	for other, support := range candidates {
		conf := float64(support) / (denom * math.Sqrt(float64(len(g.entityDocs[other]))))
		if conf >= minConfidence {
			res.Edges = append(res.Edges, Edge{Other: other, Confidence: conf, Support: support})
		}
	}
	sort.Slice(res.Edges, func(i, j int) bool { return res.Edges[i].Other < res.Edges[j].Other })
	return res
}

// SupportDocuments 返回两个实体间完整的支持文档编号集合（升序），
// 不受置信度阈值影响；同一实体或无支持时返回空切片。
func (g *Graph) SupportDocuments(a, b EntityID) []DocID {
	if a == b {
		return nil
	}
	set := g.edgeDocs[makePair(a, b)]
	out := make([]DocID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
