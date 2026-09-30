package kgalias

// AddDocument 新增或整体替换一篇文档。重复提交相同内容不会增加任何计数。
func (g *Graph) AddDocument(doc Document) {
	if _, ok := g.docs[doc.ID]; ok {
		g.removeDoc(doc.ID)
	}
	sentences := make([][]string, len(doc.Sentences))
	for i, s := range doc.Sentences {
		sentences[i] = append([]string(nil), s...)
	}
	ds := &docState{sentences: sentences}
	g.docs[doc.ID] = ds
	for _, s := range sentences {
		for _, alias := range s {
			set := g.aliasDocs[alias]
			if set == nil {
				set = make(map[int64]struct{})
				g.aliasDocs[alias] = set
			}
			set[doc.ID] = struct{}{}
		}
	}
	g.projectDoc(doc.ID, ds)
}

// DeleteDocument 删除一篇文档。
func (g *Graph) DeleteDocument(id int64) {
	g.removeDoc(id)
}

// removeDoc 移除文档及其全部投影贡献。
func (g *Graph) removeDoc(id int64) {
	ds, ok := g.docs[id]
	if !ok {
		return
	}
	g.unprojectDoc(id, ds)
	for _, s := range ds.sentences {
		for _, alias := range s {
			set := g.aliasDocs[alias]
			if set != nil {
				delete(set, id)
				if len(set) == 0 {
					delete(g.aliasDocs, alias)
				}
			}
		}
	}
	delete(g.docs, id)
}

// projectDoc 计算文档在当前绑定下的投影并累加到全局状态。
func (g *Graph) projectDoc(id int64, ds *docState) {
	entities, edges := computeProjection(ds.sentences, g.binding)
	ds.entities = entities
	ds.edges = edges
	for e := range entities {
		g.docFreq[e]++
	}
	for k := range edges {
		set := g.edgeSupport[k]
		if set == nil {
			set = make(map[int64]struct{})
			g.edgeSupport[k] = set
			g.addAdj(k)
		}
		set[id] = struct{}{}
	}
}

// unprojectDoc 从全局状态中减去文档的投影贡献。
func (g *Graph) unprojectDoc(id int64, ds *docState) {
	for e := range ds.entities {
		g.docFreq[e]--
		if g.docFreq[e] == 0 {
			delete(g.docFreq, e)
		}
	}
	for k := range ds.edges {
		set := g.edgeSupport[k]
		delete(set, id)
		if len(set) == 0 {
			delete(g.edgeSupport, k)
			g.removeAdj(k)
		}
	}
	ds.entities = nil
	ds.edges = nil
}

// addAdj 把新边登记到两端实体的邻接索引。
func (g *Graph) addAdj(k edgeKey) {
	for _, e := range [2]int64{k.a, k.b} {
		set := g.adj[e]
		if set == nil {
			set = make(map[edgeKey]struct{})
			g.adj[e] = set
		}
		set[k] = struct{}{}
	}
}

// removeAdj 把消失的边从两端实体的邻接索引中移除。
func (g *Graph) removeAdj(k edgeKey) {
	for _, e := range [2]int64{k.a, k.b} {
		set := g.adj[e]
		delete(set, k)
		if len(set) == 0 {
			delete(g.adj, e)
		}
	}
}
