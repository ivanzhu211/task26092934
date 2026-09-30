package kgalias

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func checkNode(t *testing.T, got, want NodeInfo) {
	t.Helper()
	if got.Entity != want.Entity || got.DocFreq != want.DocFreq {
		t.Fatalf("node mismatch: got %+v want %+v", got, want)
	}
	if len(got.Edges) != len(want.Edges) {
		t.Fatalf("node %d edges mismatch: got %+v want %+v", got.Entity, got.Edges, want.Edges)
	}
	for i := range got.Edges {
		g, w := got.Edges[i], want.Edges[i]
		if g.EntityA != w.EntityA || g.EntityB != w.EntityB || g.Support != w.Support {
			t.Fatalf("node %d edge %d mismatch: got %+v want %+v", got.Entity, i, g, w)
		}
		if math.Abs(g.Confidence-w.Confidence) > 1e-12 {
			t.Fatalf("node %d edge %d confidence: got %v want %v", got.Entity, i, g.Confidence, w.Confidence)
		}
	}
}

func checkDocs(t *testing.T, got, want []int64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("support docs: got %v want %v", got, want)
	}
}

// 重复提及与重复提交不产生额外计数。
func TestDuplicateMentionsAndResubmit(t *testing.T) {
	g := NewGraph()
	g.SubmitVersion(AliasVersion{ID: "v1", Bindings: map[string]int64{"a": 1, "b": 2}})
	doc := Document{ID: 1, Sentences: [][]string{{"a", "a", "b", "a"}}}
	g.AddDocument(doc)
	g.AddDocument(doc) // 重复提交相同内容
	g.AddDocument(doc)

	if n := g.Node(1); n.DocFreq != 1 {
		t.Fatalf("docfreq(1) = %d, want 1", n.DocFreq)
	}
	if n := g.Node(2); n.DocFreq != 1 {
		t.Fatalf("docfreq(2) = %d, want 1", n.DocFreq)
	}
	checkDocs(t, g.SupportDocs(1, 2), []int64{1})
}

// 句距 2 连边，句距 3 不连边；重叠窗口只贡献一次支持。
func TestWindowDistance(t *testing.T) {
	g := NewGraph()
	g.SubmitVersion(AliasVersion{ID: "v1", Bindings: map[string]int64{
		"a": 1, "b": 2, "c": 3, "d": 4,
	}})
	g.AddDocument(Document{ID: 1, Sentences: [][]string{
		{"a"}, // 句0
		{"x"}, // 句1 未绑定
		{"b"}, // 句2：与 a 相距 2 -> 边(1,2)
		{"c"}, // 句3：与 a 相距 3 -> 无边；与 b 相距 1 -> 边(2,3)
		{"d"}, // 句4：与 b 相距 2 -> 边(2,4)；与 c 相距 1 -> 边(3,4)
	}})
	for _, e := range []struct{ a, b int64 }{{1, 2}, {2, 3}, {2, 4}, {3, 4}} {
		checkDocs(t, g.SupportDocs(e.a, e.b), []int64{1})
	}
	for _, e := range [][2]int64{{1, 3}, {1, 4}} {
		checkDocs(t, g.SupportDocs(e[0], e[1]), nil)
	}
	// 重叠窗口：同一文档多个窗口覆盖同一对实体仍只计一次。
	g.AddDocument(Document{ID: 2, Sentences: [][]string{
		{"a", "b"}, {"a", "b"}, {"a", "b"},
	}})
	checkDocs(t, g.SupportDocs(1, 2), []int64{1, 2})
}

// 仅节点频次变化时，置信度筛选随之变化。
func TestConfidenceThresholdFollowsFreq(t *testing.T) {
	g := NewGraph()
	g.SubmitVersion(AliasVersion{ID: "v1", Bindings: map[string]int64{"a": 1, "b": 2, "c": 3}})
	// 实体1与2共同出现在1篇；实体2还出现在另外3篇（与3共现）。
	g.AddDocument(Document{ID: 1, Sentences: [][]string{{"a", "b"}}})
	for i := int64(2); i <= 4; i++ {
		g.AddDocument(Document{ID: i, Sentences: [][]string{{"b", "c"}}})
	}
	// freq(1)=1, freq(2)=4, support(1,2)=1 -> conf = 1/sqrt(4) = 0.5 -> 保留
	n := g.Node(1)
	if len(n.Edges) != 1 || n.Edges[0].EntityB != 2 {
		t.Fatalf("expected edge (1,2), got %+v", n.Edges)
	}
	if math.Abs(n.Edges[0].Confidence-0.5) > 1e-12 {
		t.Fatalf("confidence = %v, want 0.5", n.Edges[0].Confidence)
	}
	// 增加一篇只含 b 的文档，freq(2)=5 -> conf = 1/sqrt(5) < 0.5 -> 被过滤
	g.AddDocument(Document{ID: 5, Sentences: [][]string{{"b"}}})
	if n := g.Node(1); len(n.Edges) != 0 {
		t.Fatalf("expected no edges after freq change, got %+v", n.Edges)
	}
	// 支持文档集合查询不受阈值影响
	checkDocs(t, g.SupportDocs(1, 2), []int64{1})
	// 删除该文档后恢复
	g.DeleteDocument(5)
	if n := g.Node(1); len(n.Edges) != 1 {
		t.Fatalf("expected edge restored, got %+v", n.Edges)
	}
}

// 合并产生自环消失，拆分后恢复仍有证据的关系。
func TestMergeSplitAndUnbind(t *testing.T) {
	g := NewGraph()
	g.SubmitVersion(AliasVersion{ID: "v1", Bindings: map[string]int64{
		"a": 1, "b": 2, "c": 3,
	}})
	g.AddDocument(Document{ID: 1, Sentences: [][]string{{"a", "b"}}})
	g.AddDocument(Document{ID: 2, Sentences: [][]string{{"a", "c"}}})
	checkDocs(t, g.SupportDocs(1, 2), []int64{1})
	checkDocs(t, g.SupportDocs(1, 3), []int64{2})

	// 合并：a、b 都绑定到实体 1 -> 边(1,2) 变自环消失
	g.SubmitVersion(AliasVersion{ID: "v2", Bindings: map[string]int64{
		"a": 1, "b": 1, "c": 3,
	}})
	checkDocs(t, g.SupportDocs(1, 2), nil)
	if n := g.Node(2); n.DocFreq != 0 {
		t.Fatalf("entity 2 should vanish after merge, freq=%d", n.DocFreq)
	}
	// 合并后实体1的文档频次 = 2（文档1、2 都含实体1）
	if n := g.Node(1); n.DocFreq != 2 {
		t.Fatalf("freq(1) = %d, want 2", n.DocFreq)
	}
	// 文档2中 a 与 c 共现 -> 边(1,3) 仍存在
	checkDocs(t, g.SupportDocs(1, 3), []int64{2})

	// 合并期间替换文档1：去掉 b，加入 c
	g.AddDocument(Document{ID: 1, Sentences: [][]string{{"a", "c"}}})
	// 删除文档2
	g.DeleteDocument(2)

	// 拆回 v1：关系只能依据仍存在的当前文档恢复
	g.SwitchVersion("v1")
	checkDocs(t, g.SupportDocs(1, 2), nil)        // 文档1已不含 b，文档2已删除
	checkDocs(t, g.SupportDocs(1, 3), []int64{1}) // 文档1的 a,c 共现恢复
	if n := g.Node(2); n.DocFreq != 0 {
		t.Fatalf("entity 2 should have no docs, freq=%d", n.DocFreq)
	}

	// 暂时解绑 b：提及 b 的文档重新加入后可再参与
	g.AddDocument(Document{ID: 3, Sentences: [][]string{{"a", "b"}}})
	g.SubmitVersion(AliasVersion{ID: "v3", Bindings: map[string]int64{
		"a": 1, "c": 3, // b 未绑定
	}})
	checkDocs(t, g.SupportDocs(1, 2), nil)
	if n := g.Node(2); n.DocFreq != 0 {
		t.Fatalf("entity 2 should vanish while b unbound, freq=%d", n.DocFreq)
	}
	// 重新绑定 b -> 文档3的提及重新参与
	g.SubmitVersion(AliasVersion{ID: "v4", Bindings: map[string]int64{
		"a": 1, "b": 2, "c": 3,
	}})
	checkDocs(t, g.SupportDocs(1, 2), []int64{3})
}

// 随机混合操作后，与暴力重算逐次核对。
func TestRandomMixedOpsMatchReference(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	g := NewGraph()
	ref := newReference()

	aliases := []string{"a", "b", "c", "d", "e", "f"}
	versions := []map[string]int64{
		{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6},
		{"a": 1, "b": 1, "c": 3, "d": 4, "e": 5, "f": 6}, // 合并 a,b
		{"a": 1, "b": 2, "c": 3, "d": 3, "e": 5},         // 合并 c,d，解绑 f
		{"a": 7, "b": 7, "c": 7, "d": 4, "e": 5, "f": 6}, // 三向合并
	}
	for i, v := range versions {
		id := string(rune('A' + i))
		g.SubmitVersion(AliasVersion{ID: id, Bindings: v})
		ref.binding = v
	}

	verify := func(step int) {
		for e := int64(1); e <= 7; e++ {
			checkNode(t, g.Node(e), ref.node(e))
			for o := e + 1; o <= 7; o++ {
				got := g.SupportDocs(e, o)
				want := ref.supportDocs(e, o)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("step %d: SupportDocs(%d,%d) got %v want %v", step, e, o, got, want)
				}
			}
		}
	}

	for step := 0; step < 300; step++ {
		switch rng.Intn(5) {
		case 0, 1: // 新增/替换文档
			id := int64(rng.Intn(30))
			ns := 1 + rng.Intn(6)
			doc := Document{ID: id, Sentences: make([][]string, ns)}
			for i := range doc.Sentences {
				nm := rng.Intn(4)
				for j := 0; j < nm; j++ {
					doc.Sentences[i] = append(doc.Sentences[i], aliases[rng.Intn(len(aliases))])
				}
			}
			g.AddDocument(doc)
			ref.docs[id] = doc.Sentences
		case 2: // 删除文档
			id := int64(rng.Intn(30))
			g.DeleteDocument(id)
			delete(ref.docs, id)
		case 3, 4: // 切换版本
			i := rng.Intn(len(versions))
			id := string(rune('A' + i))
			g.SwitchVersion(id)
			ref.binding = versions[i]
		}
		verify(step)
	}
}
