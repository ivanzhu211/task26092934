package kgalias

// windowSize 是共现窗口：句子序号相差不超过 2 即构成支持。
const windowSize = 2

// entityPair 是一条无向边的键，保证 A < B，不产生自环。
type entityPair struct {
	A, B EntityID
}

func makePair(a, b EntityID) entityPair {
	if a > b {
		a, b = b, a
	}
	return entityPair{A: a, B: b}
}

// docContribution 是一篇文档在当前别名版本下对图谱的贡献。
type docContribution struct {
	// entities 是文档中出现的实体集合（重复提及与多个别名只计一次）。
	entities map[EntityID]struct{}
	// pairs 是文档支持的实体对集合（无论多少提及组合只计一次）。
	pairs map[entityPair]struct{}
}

// computeContribution 依据给定绑定计算一篇文档的贡献。
// 未绑定的别名不参与；同一实体的多个提及只保留句子序号用于窗口判断。
func computeContribution(sentences [][]Alias, bindings map[Alias]EntityID) docContribution {
	// 每句的实体集合。
	sentEntities := make([]map[EntityID]struct{}, len(sentences))
	entities := make(map[EntityID]struct{})
	for i, mentions := range sentences {
		set := make(map[EntityID]struct{}, len(mentions))
		for _, m := range mentions {
			e, ok := bindings[m]
			if !ok {
				continue
			}
			set[e] = struct{}{}
			entities[e] = struct{}{}
		}
		sentEntities[i] = set
	}

	pairs := make(map[entityPair]struct{})
	for i, set := range sentEntities {
		// 同句共现。
		for a := range set {
			for b := range set {
				if a < b {
					pairs[entityPair{A: a, B: b}] = struct{}{}
				}
			}
		}
		// 与后续 windowSize 句内的实体共现。
		for j := i + 1; j < len(sentEntities) && j-i <= windowSize; j++ {
			for a := range set {
				for b := range sentEntities[j] {
					if a != b {
						pairs[makePair(a, b)] = struct{}{}
					}
				}
			}
		}
	}
	return docContribution{entities: entities, pairs: pairs}
}
