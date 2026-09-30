package kgalias

// Document 是一篇文档的输入：按顺序排列的句子，每句是别名提及列表。
type Document struct {
	ID        int64
	Sentences [][]string
}

// AliasVersion 是一个完整的别名版本：别名 -> 实体编号。
type AliasVersion struct {
	ID       string
	Bindings map[string]int64
}

// EdgeInfo 描述一条邻边。
type EdgeInfo struct {
	EntityA    int64
	EntityB    int64
	Support    int
	Confidence float64
}

// NodeInfo 是邻居查询结果。
type NodeInfo struct {
	Entity  int64
	DocFreq int
	Edges   []EdgeInfo
}
