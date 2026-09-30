package kgalias

// SubmitVersion 提交并启用一个新的完整别名版本。版本 ID 不可重复。
func (g *Graph) SubmitVersion(v AliasVersion) {
	if _, ok := g.versions[v.ID]; ok {
		panic("kgalias: version already registered: " + v.ID)
	}
	bindings := make(map[string]int64, len(v.Bindings))
	for alias, e := range v.Bindings {
		bindings[alias] = e
	}
	g.versions[v.ID] = bindings
	g.applyVersion(v.ID)
}

// SwitchVersion 切换到已登记的版本。
func (g *Graph) SwitchVersion(id string) {
	if _, ok := g.versions[id]; !ok {
		panic("kgalias: unknown version: " + id)
	}
	if id == g.current {
		return
	}
	g.applyVersion(id)
}

// applyVersion 增量地把当前绑定切换到目标版本：
// 只对绑定发生变化的别名所涉及的文档做反投影与重投影。
func (g *Graph) applyVersion(id string) {
	next := g.versions[id]

	affected := make(map[int64]struct{})
	// 旧绑定中有、且与新绑定不同的别名。
	for alias, oldE := range g.binding {
		if newE, ok := next[alias]; !ok || newE != oldE {
			for docID := range g.aliasDocs[alias] {
				affected[docID] = struct{}{}
			}
		}
	}
	// 新绑定中有、但旧绑定中没有的别名。
	for alias := range next {
		if _, ok := g.binding[alias]; !ok {
			for docID := range g.aliasDocs[alias] {
				affected[docID] = struct{}{}
			}
		}
	}

	for docID := range affected {
		ds := g.docs[docID]
		g.unprojectDoc(docID, ds)
	}
	g.current = id
	g.binding = next
	for docID := range affected {
		ds := g.docs[docID]
		g.projectDoc(docID, ds)
	}
}
