// Package kgalias 提供知识图谱别名投影能力：
// 在进程内存中维护文档、别名版本与图谱状态，
// 使图谱始终等价于用当前文档与当前别名版本从头构图。
package kgalias

// DocID 是文档的稳定编号。
type DocID string

// Alias 是文档中被提及的别名。
type Alias string

// EntityID 是实体编号，由别名版本中的绑定关系指定。
type EntityID uint64

// VersionID 是别名版本的编号。
type VersionID string

// Edge 表示一条无向边的查询结果。
type Edge struct {
	// Other 是邻居实体。
	Other EntityID
	// Confidence 为支持文档数除以两端文档频次乘积的平方根。
	Confidence float64
	// Support 是支持该边的文档数。
	Support int
}

// NeighborResult 是邻居查询的结果。
type NeighborResult struct {
	// DocFreq 是该实体的文档频次（包含该实体的不同文档数）。
	DocFreq int
	// Edges 为置信度至少 0.5 的邻边。
	Edges []Edge
}
