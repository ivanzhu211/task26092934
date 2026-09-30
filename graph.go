package kgalias

import "errors"

// ErrVersionExists 表示重复登记已存在的别名版本。
var ErrVersionExists = errors.New("kgalias: version already registered")

// ErrVersionNotFound 表示切换到未登记的别名版本。
var ErrVersionNotFound = errors.New("kgalias: version not registered")

// storedDoc 是已登记文档的原始内容与当前贡献。
type storedDoc struct {
	sentences [][]Alias
	contrib   docContribution
}

// Graph 保存文档、别名版本与投影后的图谱状态。
// 所有查询等价于使用当前文档和当前别名版本从头构图。
// 调用按顺序执行，无需并发安全。
type Graph struct {
	docs map[DocID]*storedDoc
	// aliasDocs 是别名到文档的倒排索引，与别名版本无关。
	aliasDocs map[Alias]map[DocID]struct{}
	// entityDocs 是当前版本下每个实体的支持文档集合。
	entityDocs map[EntityID]map[DocID]struct{}
	// edgeDocs 是当前版本下每条无向边的支持文档集合。
	edgeDocs map[entityPair]map[DocID]struct{}
	// versions 保存已登记、不可修改的别名版本。
	versions  map[VersionID]map[Alias]EntityID
	active    map[Alias]EntityID
	activeID  VersionID
	hasActive bool
}

// NewGraph 创建空的图谱投影。
func NewGraph() *Graph {
	return &Graph{
		docs:       make(map[DocID]*storedDoc),
		aliasDocs:  make(map[Alias]map[DocID]struct{}),
		entityDocs: make(map[EntityID]map[DocID]struct{}),
		edgeDocs:   make(map[entityPair]map[DocID]struct{}),
		versions:   make(map[VersionID]map[Alias]EntityID),
	}
}

// UpsertDocument 新增或整体替换一篇文档。
// sentences 按顺序排列，每个元素是一句中的别名提及列表。
// 重复提交相同内容先移除旧贡献再按当前版本重新计算，不改变任何计数。
func (g *Graph) UpsertDocument(id DocID, sentences [][]Alias) {
	if old, ok := g.docs[id]; ok {
		g.removeDoc(id, old)
	}
	copied := make([][]Alias, len(sentences))
	for i, mentions := range sentences {
		copied[i] = append([]Alias(nil), mentions...)
	}
	doc := &storedDoc{sentences: copied}
	if g.hasActive {
		doc.contrib = computeContribution(copied, g.active)
		g.applyContribution(id, doc.contrib)
	}
	g.docs[id] = doc
	for _, mentions := range copied {
		for _, m := range mentions {
			set := g.aliasDocs[m]
			if set == nil {
				set = make(map[DocID]struct{})
				g.aliasDocs[m] = set
			}
			set[id] = struct{}{}
		}
	}
}

// DeleteDocument 删除一篇文档；删除不存在的文档不产生任何效果。
func (g *Graph) DeleteDocument(id DocID) {
	doc, ok := g.docs[id]
	if !ok {
		return
	}
	g.removeDoc(id, doc)
}

// removeDoc 移除文档的投影贡献与倒排索引。
func (g *Graph) removeDoc(id DocID, doc *storedDoc) {
	if g.hasActive {
		g.unapplyContribution(id, doc.contrib)
	}
	for _, mentions := range doc.sentences {
		for _, m := range mentions {
			set := g.aliasDocs[m]
			if set == nil {
				continue
			}
			delete(set, id)
			if len(set) == 0 {
				delete(g.aliasDocs, m)
			}
		}
	}
	delete(g.docs, id)
}

// applyContribution 把一篇文档的贡献加入投影。
func (g *Graph) applyContribution(id DocID, c docContribution) {
	for e := range c.entities {
		set := g.entityDocs[e]
		if set == nil {
			set = make(map[DocID]struct{})
			g.entityDocs[e] = set
		}
		set[id] = struct{}{}
	}
	for p := range c.pairs {
		set := g.edgeDocs[p]
		if set == nil {
			set = make(map[DocID]struct{})
			g.edgeDocs[p] = set
		}
		set[id] = struct{}{}
	}
}

// unapplyContribution 把一篇文档的贡献从投影中移除。
func (g *Graph) unapplyContribution(id DocID, c docContribution) {
	for e := range c.entities {
		set := g.entityDocs[e]
		if set == nil {
			continue
		}
		delete(set, id)
		if len(set) == 0 {
			delete(g.entityDocs, e)
		}
	}
	for p := range c.pairs {
		set := g.edgeDocs[p]
		if set == nil {
			continue
		}
		delete(set, id)
		if len(set) == 0 {
			delete(g.edgeDocs, p)
		}
	}
}

// RegisterVersion 登记一个完整且不可修改的别名版本。
// bindings 将别名绑定到实体编号，允许多个别名绑定同一实体。
// 已存在的版本不可重复登记。
func (g *Graph) RegisterVersion(id VersionID, bindings map[Alias]EntityID) error {
	if _, ok := g.versions[id]; ok {
		return ErrVersionExists
	}
	copied := make(map[Alias]EntityID, len(bindings))
	for a, e := range bindings {
		copied[a] = e
	}
	g.versions[id] = copied
	return nil
}

// ActivateVersion 切换到已登记的别名版本。
// 只重算绑定发生变化的别名所涉及的文档，不恢复旧文档、
// 不撤销期间的文档修改。
func (g *Graph) ActivateVersion(id VersionID) error {
	next, ok := g.versions[id]
	if !ok {
		return ErrVersionNotFound
	}
	if g.hasActive && g.activeID == id {
		return nil
	}
	// 找出绑定发生变化的别名。
	changed := make(map[Alias]struct{})
	for a, e := range next {
		if old, ok := g.active[a]; !ok || old != e {
			changed[a] = struct{}{}
		}
	}
	for a := range g.active {
		if _, ok := next[a]; !ok {
			changed[a] = struct{}{}
		}
	}
	// 受影响文档为包含任一变化别名的文档，每篇只重算一次。
	affected := make(map[DocID]struct{})
	for a := range changed {
		for docID := range g.aliasDocs[a] {
			affected[docID] = struct{}{}
		}
	}
	if g.hasActive {
		for docID := range affected {
			doc := g.docs[docID]
			g.unapplyContribution(docID, doc.contrib)
		}
	}
	g.active = next
	g.activeID = id
	g.hasActive = true
	for docID := range affected {
		doc := g.docs[docID]
		doc.contrib = computeContribution(doc.sentences, next)
		g.applyContribution(docID, doc.contrib)
	}
	return nil
}

// ActiveVersion 返回当前启用的别名版本；尚未启用时返回空串与 false。
func (g *Graph) ActiveVersion() (VersionID, bool) {
	return g.activeID, g.hasActive
}
