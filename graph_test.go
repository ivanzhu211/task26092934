package kgalias

import (
	"math"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// reference 是测试用的朴素参照：用当前文档与当前绑定从头构图。
type reference struct {
	entityDocs map[EntityID]map[DocID]struct{}
	edgeDocs   map[entityPair]map[DocID]struct{}
}

func recompute(docs map[DocID][][]Alias, bindings map[Alias]EntityID) reference {
	r := reference{
		entityDocs: make(map[EntityID]map[DocID]struct{}),
		edgeDocs:   make(map[entityPair]map[DocID]struct{}),
	}
	for id, sentences := range docs {
		c := computeContribution(sentences, bindings)
		for e := range c.entities {
			if r.entityDocs[e] == nil {
				r.entityDocs[e] = make(map[DocID]struct{})
			}
			r.entityDocs[e][id] = struct{}{}
		}
		for p := range c.pairs {
			if r.edgeDocs[p] == nil {
				r.edgeDocs[p] = make(map[DocID]struct{})
			}
			r.edgeDocs[p][id] = struct{}{}
		}
	}
	return r
}

// assertConsistent 校验图谱内部状态与朴素重算完全一致。
func assertConsistent(t *testing.T, g *Graph, docs map[DocID][][]Alias, bindings map[Alias]EntityID) {
	t.Helper()
	want := recompute(docs, bindings)
	if !reflect.DeepEqual(g.entityDocs, want.entityDocs) {
		t.Fatalf("entityDocs mismatch\ngot:  %v\nwant: %v", g.entityDocs, want.entityDocs)
	}
	if !reflect.DeepEqual(g.edgeDocs, want.edgeDocs) {
		t.Fatalf("edgeDocs mismatch\ngot:  %v\nwant: %v", g.edgeDocs, want.edgeDocs)
	}
	// 通过公开查询再核对一遍。
	for e, docSet := range want.entityDocs {
		if got := g.DocumentFrequency(e); got != len(docSet) {
			t.Fatalf("DocumentFrequency(%d) = %d, want %d", e, got, len(docSet))
		}
	}
	for p, docSet := range want.edgeDocs {
		got := g.SupportDocuments(p.A, p.B)
		wantDocs := make([]DocID, 0, len(docSet))
		for id := range docSet {
			wantDocs = append(wantDocs, id)
		}
		sort.Slice(wantDocs, func(i, j int) bool { return wantDocs[i] < wantDocs[j] })
		if !reflect.DeepEqual(got, wantDocs) {
			t.Fatalf("SupportDocuments(%d,%d) = %v, want %v", p.A, p.B, got, wantDocs)
		}
	}
}

func TestRepeatedMentionsAndWindow(t *testing.T) {
	g := NewGraph()
	mustRegister(t, g, "v1", map[Alias]EntityID{"a": 1, "b": 2, "c": 3})
	mustActivate(t, g, "v1")

	// 同句重复提及同一别名；句距 2 构成支持，句距 3 不构成。
	g.UpsertDocument("d1", [][]Alias{
		{"a", "a", "b"}, // 句0：a 重复提及，a-b 同句
		{"x"},           // 句1：未绑定别名，不参与
		{"c"},           // 句2：与句0 相距 2，a-c、b-c 均成立
	})
	assertSupport(t, g, 1, 2, "d1")
	assertSupport(t, g, 1, 3, "d1")
	assertSupport(t, g, 2, 3, "d1")
	assertFreq(t, g, 1, 1)
	assertFreq(t, g, 2, 1)
	assertFreq(t, g, 3, 1)

	// 另一篇文档：句距 3 不构成支持。
	g.UpsertDocument("d2", [][]Alias{
		{"a"},
		{},
		{},
		{"c"}, // 句3：与句0 相距 3
	})
	assertSupport(t, g, 1, 3, "d1") // d2 不贡献
	assertFreq(t, g, 1, 2)
	assertFreq(t, g, 3, 2)

	// 重叠窗口：同一文档多个提及组合只贡献一次。
	g.UpsertDocument("d3", [][]Alias{
		{"a"}, {"b"}, {"a"}, {"b"}, {"a"},
	})
	assertSupport(t, g, 1, 2, "d1", "d3")
	assertFreq(t, g, 1, 3)
	assertFreq(t, g, 2, 2)
}

func TestIdempotentUpsert(t *testing.T) {
	g := NewGraph()
	mustRegister(t, g, "v1", map[Alias]EntityID{"a": 1, "b": 2})
	mustActivate(t, g, "v1")
	sentences := [][]Alias{{"a", "b"}, {"a"}}
	g.UpsertDocument("d1", sentences)
	g.UpsertDocument("d1", sentences) // 重复提交相同内容
	g.UpsertDocument("d1", [][]Alias{{"a", "b"}, {"a"}})
	assertFreq(t, g, 1, 1)
	assertFreq(t, g, 2, 1)
	assertSupport(t, g, 1, 2, "d1")
}

func TestConfidenceThresholdFollowsDocFreq(t *testing.T) {
	g := NewGraph()
	mustRegister(t, g, "v1", map[Alias]EntityID{"a": 1, "b": 2})
	mustActivate(t, g, "v1")

	// d1..d4 均支持边 1-2；实体 1 出现在 4 篇，实体 2 出现在 4 篇。
	for _, id := range []DocID{"d1", "d2", "d3", "d4"} {
		g.UpsertDocument(id, [][]Alias{{"a", "b"}})
	}
	// 置信度 = 4 / sqrt(4*4) = 1.0，可见。
	assertNeighbor(t, g, 1, 2, 4, 1.0)

	// 让实体 2 额外出现在 12 篇不含 a 的文档中：
	// 支持不变，置信度 = 4 / sqrt(4*16) = 0.5，仍可见。
	for i := 0; i < 12; i++ {
		g.UpsertDocument(DocID("extra"+string(rune('a'+i))), [][]Alias{{"b"}})
	}
	assertNeighbor(t, g, 1, 2, 4, 0.5)

	// 再增加一篇只含 b 的文档：置信度 = 4/sqrt(4*17) ≈ 0.485 < 0.5，被过滤。
	g.UpsertDocument("extra_z", [][]Alias{{"b"}})
	res := g.Neighbors(1)
	if len(res.Edges) != 0 {
		t.Fatalf("expected no edges above threshold, got %+v", res.Edges)
	}
	// 完整支持集合查询不受阈值影响。
	assertSupport(t, g, 1, 2, "d1", "d2", "d3", "d4")
}

func TestMergeSplitAndUnbind(t *testing.T) {
	g := NewGraph()
	// v1：a、b 分属实体 1、2。
	mustRegister(t, g, "v1", map[Alias]EntityID{"a": 1, "b": 2, "c": 3})
	// v2：a、b 合并为实体 1。
	mustRegister(t, g, "v2", map[Alias]EntityID{"a": 1, "b": 1, "c": 3})
	// v3：a 暂时解绑。
	mustRegister(t, g, "v3", map[Alias]EntityID{"b": 2, "c": 3})
	mustActivate(t, g, "v1")

	g.UpsertDocument("d1", [][]Alias{{"a"}, {"b"}}) // 句距1，支持 1-2
	g.UpsertDocument("d2", [][]Alias{{"a", "c"}})   // 支持 1-3
	g.UpsertDocument("d3", [][]Alias{{"b", "c"}})   // 支持 2-3
	assertSupport(t, g, 1, 2, "d1")

	// 合并：1-2 变成自环消失；d1 中 b 归入实体 1，d1 开始支持 1-3？否——
	// d1 只有 a、b，没有 c。d2 支持 1-3，d3 中 b 归入 1，支持 1-3。
	mustActivate(t, g, "v2")
	assertSupport(t, g, 1, 2) // 自环消失
	assertSupport(t, g, 1, 3, "d2", "d3")
	assertFreq(t, g, 1, 3) // d1(a,b)、d2(a)、d3(b)

	// 合并期间替换 d3：去掉 b，改为只含 c。
	g.UpsertDocument("d3", [][]Alias{{"c"}})
	assertSupport(t, g, 1, 3, "d2")
	assertFreq(t, g, 1, 2)

	// 暂时解绑 a：实体 1 只剩 b 的文档；a 的提及不再参与。
	mustActivate(t, g, "v3")
	assertFreq(t, g, 1, 0)
	assertFreq(t, g, 2, 1) // d1 的 b
	assertSupport(t, g, 2, 3)

	// 解绑期间删除 d2、替换 d1 去掉 b。
	g.DeleteDocument("d2")
	g.UpsertDocument("d1", [][]Alias{{"a"}})
	assertFreq(t, g, 2, 0)

	// 切回 v1：关系只能依据仍存在的当前文档恢复。
	// d1 现为 {a}，d2 已删，d3 现为 {c}。
	mustActivate(t, g, "v1")
	assertFreq(t, g, 1, 1) // d1
	assertFreq(t, g, 3, 1) // d3
	assertFreq(t, g, 2, 0)
	assertSupport(t, g, 1, 2)
	assertSupport(t, g, 1, 3)
}

func TestVersionImmutabilityAndErrors(t *testing.T) {
	g := NewGraph()
	mustRegister(t, g, "v1", map[Alias]EntityID{"a": 1})
	if err := g.RegisterVersion("v1", map[Alias]EntityID{"a": 9}); err != ErrVersionExists {
		t.Fatalf("expected ErrVersionExists, got %v", err)
	}
	if err := g.ActivateVersion("nope"); err != ErrVersionNotFound {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
	// 调用方事后修改传入的 bindings 不影响已登记版本。
	b := map[Alias]EntityID{"x": 5}
	mustRegister(t, g, "v2", b)
	b["x"] = 6
	mustActivate(t, g, "v2")
	g.UpsertDocument("d1", [][]Alias{{"x"}})
	assertFreq(t, g, 5, 1)
	assertFreq(t, g, 6, 0)
}

// TestMixedOpsAgainstReference 连续混合修改，逐次与完整重算核对。
func TestMixedOpsAgainstReference(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	g := NewGraph()
	docs := make(map[DocID][][]Alias)

	aliases := []Alias{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"}
	bindingsV1 := map[Alias]EntityID{
		"a0": 1, "a1": 2, "a2": 3, "a3": 4, "a4": 5, "a5": 6, "a6": 7, "a7": 8,
	}
	// v2 合并部分实体并解绑 a7。
	bindingsV2 := map[Alias]EntityID{
		"a0": 1, "a1": 1, "a2": 3, "a3": 3, "a4": 5, "a5": 6, "a6": 6,
	}
	mustRegister(t, g, "v1", bindingsV1)
	mustRegister(t, g, "v2", bindingsV2)
	mustActivate(t, g, "v1")
	active := bindingsV1
	onV1 := true

	randomDoc := func() [][]Alias {
		n := 1 + rng.Intn(6)
		sentences := make([][]Alias, n)
		for i := range sentences {
			m := rng.Intn(4)
			mentions := make([]Alias, m)
			for j := range mentions {
				mentions[j] = aliases[rng.Intn(len(aliases))]
			}
			sentences[i] = mentions
		}
		return sentences
	}

	for step := 0; step < 400; step++ {
		switch rng.Intn(5) {
		case 0, 1: // 新增或替换
			id := DocID("doc" + string(rune('A'+rng.Intn(40))))
			s := randomDoc()
			g.UpsertDocument(id, s)
			docs[id] = s
		case 2: // 删除
			if len(docs) > 0 {
				keys := make([]DocID, 0, len(docs))
				for k := range docs {
					keys = append(keys, k)
				}
				id := keys[rng.Intn(len(keys))]
				g.DeleteDocument(id)
				delete(docs, id)
			}
		case 3: // 切换版本
			if onV1 {
				mustActivate(t, g, "v2")
				active = bindingsV2
			} else {
				mustActivate(t, g, "v1")
				active = bindingsV1
			}
			onV1 = !onV1
		case 4: // 重复提交相同内容
			if len(docs) > 0 {
				keys := make([]DocID, 0, len(docs))
				for k := range docs {
					keys = append(keys, k)
				}
				id := keys[rng.Intn(len(keys))]
				g.UpsertDocument(id, docs[id])
			}
		}
		assertConsistent(t, g, docs, active)
	}
}

func mustRegister(t *testing.T, g *Graph, id VersionID, bindings map[Alias]EntityID) {
	t.Helper()
	if err := g.RegisterVersion(id, bindings); err != nil {
		t.Fatalf("RegisterVersion(%s): %v", id, err)
	}
}

func mustActivate(t *testing.T, g *Graph, id VersionID) {
	t.Helper()
	if err := g.ActivateVersion(id); err != nil {
		t.Fatalf("ActivateVersion(%s): %v", id, err)
	}
}

func assertFreq(t *testing.T, g *Graph, e EntityID, want int) {
	t.Helper()
	if got := g.DocumentFrequency(e); got != want {
		t.Fatalf("DocumentFrequency(%d) = %d, want %d", e, got, want)
	}
}

func assertSupport(t *testing.T, g *Graph, a, b EntityID, want ...DocID) {
	t.Helper()
	got := g.SupportDocuments(a, b)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if len(want) == 0 {
		want = nil
	}
	if len(got) == 0 {
		got = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportDocuments(%d,%d) = %v, want %v", a, b, got, want)
	}
}

func assertNeighbor(t *testing.T, g *Graph, e, other EntityID, wantSupport int, wantConf float64) {
	t.Helper()
	res := g.Neighbors(e)
	for _, edge := range res.Edges {
		if edge.Other == other {
			if edge.Support != wantSupport {
				t.Fatalf("edge %d-%d support = %d, want %d", e, other, edge.Support, wantSupport)
			}
			if math.Abs(edge.Confidence-wantConf) > 1e-9 {
				t.Fatalf("edge %d-%d confidence = %v, want %v", e, other, edge.Confidence, wantConf)
			}
			return
		}
	}
	t.Fatalf("edge %d-%d not found in neighbors %+v", e, other, res.Edges)
}
