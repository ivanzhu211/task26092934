package kgalias

// windowSize 是共现窗口的句距上限。
const windowSize = 2

// computeProjection 计算单篇文档在给定绑定下的实体集合与边集合。
// 同一句或句距 <= windowSize 的两个不同实体的任意提及产生一条无向边；
// 同一文档对同一条边只贡献一次；不产生自环。
func computeProjection(sentences [][]string, binding map[string]int64) (map[int64]struct{}, map[edgeKey]struct{}) {
	entities := make(map[int64]struct{})
	edges := make(map[edgeKey]struct{})

	// sentEntities[i] 是第 i 句提及解析到的实体集合（句内去重）。
	sentEntities := make([][]int64, len(sentences))
	for i, sent := range sentences {
		seen := make(map[int64]struct{})
		for _, alias := range sent {
			e, ok := binding[alias]
			if !ok {
				continue
			}
			if _, dup := seen[e]; !dup {
				seen[e] = struct{}{}
				sentEntities[i] = append(sentEntities[i], e)
			}
		}
		for e := range seen {
			entities[e] = struct{}{}
		}
	}

	// 对每个起始句 i，收集窗口 [i, i+windowSize] 内的实体并两两连边。
	for i := range sentEntities {
		window := make(map[int64]struct{})
		end := i + windowSize
		if end > len(sentEntities)-1 {
			end = len(sentEntities) - 1
		}
		for j := i; j <= end; j++ {
			for _, e := range sentEntities[j] {
				window[e] = struct{}{}
			}
		}
		list := make([]int64, 0, len(window))
		for e := range window {
			list = append(list, e)
		}
		for x := 0; x < len(list); x++ {
			for y := x + 1; y < len(list); y++ {
				edges[makeEdgeKey(list[x], list[y])] = struct{}{}
			}
		}
	}
	return entities, edges
}
