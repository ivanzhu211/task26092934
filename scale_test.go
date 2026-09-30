package kgalias

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// buildCorpus 生成 nDocs 篇文档，每篇约 mentionsPerDoc 次提及，
// 别名取自 aliases。返回实际提及总数。
func buildCorpus(g *Graph, rng *rand.Rand, startID int64, nDocs int, aliases []string, mentionsPerDoc int) int {
	total := 0
	for i := 0; i < nDocs; i++ {
		nSent := 1 + mentionsPerDoc/2
		doc := Document{ID: startID + int64(i), Sentences: make([][]string, nSent)}
		remaining := mentionsPerDoc
		for s := range doc.Sentences {
			nm := 0
			if s == nSent-1 {
				nm = remaining
			} else if remaining > 0 {
				nm = rng.Intn(remaining + 1)
			}
			for j := 0; j < nm; j++ {
				doc.Sentences[s] = append(doc.Sentences[s], aliases[rng.Intn(len(aliases))])
			}
			total += nm
			remaining -= nm
		}
		g.AddDocument(doc)
	}
	return total
}

func percentile(d []time.Duration, p float64) time.Duration {
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(p*float64(len(s))) - 1
	if idx < 0 {
		idx = 0
	}
	return s[idx]
}

func median(d []time.Duration) time.Duration { return percentile(d, 0.5) }

// 规模验证：10万文档、200万提及、2万别名；
// 单绑定差异的版本切换 P95 <= 100ms；
// 语料 1万 -> 10万 后切换耗时中位数不超过原来 3 倍；
// 邻边 <= 100 的节点邻居查询 P95 <= 30ms。
func TestScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}
	rng := rand.New(rand.NewSource(7))

	const nAliases = 20000
	aliases := make([]string, nAliases)
	bindingsV1 := make(map[string]int64, nAliases+1)
	bindingsV2 := make(map[string]int64, nAliases+1)
	for i := 0; i < nAliases; i++ {
		aliases[i] = fmt.Sprintf("alias-%d", i)
		bindingsV1[aliases[i]] = int64(i + 1)
		bindingsV2[aliases[i]] = int64(i + 1)
	}
	// 两个版本仅差一条绑定：special 绑定到不同实体。
	bindingsV1["special"] = 900001
	bindingsV2["special"] = 900002

	g := NewGraph()
	g.SubmitVersion(AliasVersion{ID: "v1", Bindings: bindingsV1})
	g.SubmitVersion(AliasVersion{ID: "v2", Bindings: bindingsV2})
	g.SwitchVersion("v1")

	// 1 万篇文档语料。
	mentions := buildCorpus(g, rng, 0, 10000, aliases, 20)
	// special 只出现在 20 篇文档中。
	for i := int64(10000); i < 10020; i++ {
		g.AddDocument(Document{ID: i, Sentences: [][]string{{"special", aliases[rng.Intn(nAliases)]}}})
		mentions += 2
	}

	measureSwitch := func(iters int) []time.Duration {
		d := make([]time.Duration, 0, iters)
		for i := 0; i < iters; i++ {
			from, to := "v1", "v2"
			if i%2 == 1 {
				from, to = to, from
			}
			_ = from
			start := time.Now()
			g.SwitchVersion(to)
			d = append(d, time.Since(start))
		}
		return d
	}

	small := measureSwitch(50)
	t.Logf("10k docs: switch median=%v p95=%v", median(small), percentile(small, 0.95))
	if p95 := percentile(small, 0.95); p95 > 100*time.Millisecond {
		t.Fatalf("switch p95 at 10k docs = %v, want <= 100ms", p95)
	}

	// 扩至 10 万篇：新增文档与修改涉及的别名、实体完全无关。
	mentions += buildCorpus(g, rng, 20000, 90000, aliases, 20)
	t.Logf("total mentions = %d (target ~2M)", mentions)

	large := measureSwitch(50)
	t.Logf("100k docs: switch median=%v p95=%v", median(large), percentile(large, 0.95))
	if p95 := percentile(large, 0.95); p95 > 100*time.Millisecond {
		t.Fatalf("switch p95 at 100k docs = %v, want <= 100ms", p95)
	}
	if median(large) > 3*median(small) {
		t.Fatalf("switch median grew from %v to %v (>3x)", median(small), median(large))
	}

	// 邻居查询：找一个原始邻边 <= 100 的节点。
	var target int64 = -1
	for e, edges := range g.adj {
		if len(edges) > 0 && len(edges) <= 100 {
			target = e
			break
		}
	}
	if target < 0 {
		t.Fatal("no node with <=100 raw edges found")
	}
	const q = 200
	qt := make([]time.Duration, 0, q)
	for i := 0; i < q; i++ {
		start := time.Now()
		g.Node(target)
		qt = append(qt, time.Since(start))
	}
	t.Logf("node query (edges<=%d): p95=%v", len(g.adj[target]), percentile(qt, 0.95))
	if p95 := percentile(qt, 0.95); p95 > 30*time.Millisecond {
		t.Fatalf("node query p95 = %v, want <= 30ms", p95)
	}
}
