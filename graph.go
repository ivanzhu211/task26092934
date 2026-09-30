package kgalias

// edgeKey 是无向边键，a < b。
type edgeKey struct {
	a, b int64
}

func makeEdgeKey(x, y int64) edgeKey {
	if x < y {
		return edgeKey{x, y}
	}
	return edgeKey{y, x}
}

// docState 保存一篇文档的原始输入与其在当前版本下的投影结果。
type docState struct {
	sentences [][]string
	entities  map[int64]struct{}   // 该文档投影出的实体集合
	edges     map[edgeKey]struct{} // 该文档支持的边集合
}

// Graph 保存全部进程内状态。
type Graph struct {
	docs        map[int64]*docState
	versions    map[string]map[string]int64
	current     string
	binding     map[string]int64               // 当前版本的 别名->实体
	docFreq     map[int64]int                  // 实体 -> 文档频次
	edgeSupport map[edgeKey]map[int64]struct{} // 边 -> 支持文档集合
	adj         map[int64]map[edgeKey]struct{} // 实体 -> 关联边集合
	aliasDocs   map[string]map[int64]struct{}  // 别名 -> 包含它的文档集合
}

// NewGraph 创建空图谱。
func NewGraph() *Graph {
	return &Graph{
		docs:        make(map[int64]*docState),
		versions:    make(map[string]map[string]int64),
		binding:     make(map[string]int64),
		docFreq:     make(map[int64]int),
		edgeSupport: make(map[edgeKey]map[int64]struct{}),
		adj:         make(map[int64]map[edgeKey]struct{}),
		aliasDocs:   make(map[string]map[int64]struct{}),
	}
}
