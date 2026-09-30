package kgalias

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// scaleCorpus 是规模验证用的语料参数。
type scaleCorpus struct {
	docs         int
	aliases      int
	mentionsPer  int
	hotDocs      int
	neighborSeed EntityID
}

// buildScaleGraph 构造指定规模的图谱：
// 常规别名随机分布，特殊别名 "hot" 只出现在 hotDocs 篇文档中，
// 两个版本仅在 "hot" 的绑定上不同。
func buildScaleGraph(t testing.TB, cfg scaleCorpus) *Graph {
	t.Helper()
	rng := rand.New(rand.NewSource(7))
	g := NewGraph()

	bindings := make(map[Alias]EntityID, cfg.aliases)
	for i := 0; i < cfg.aliases; i++ {
		bindings[Alias(fmt.Sprintf("al%d", i))] = EntityID(i % (cfg.aliases / 2))
	}
	// 前 100 个稀有别名绑定到独立实体，避免与随机池取模碰撞。
	for i := 0; i < 100; i++ {
		bindings[Alias(fmt.Sprintf("al%d", i))] = EntityID(50_000 + i)
	}
	v1 := make(map[Alias]EntityID, len(bindings)+1)
	for a, e := range bindings {
		v1[a] = e
	}
	v1["hot"] = EntityID(cfg.aliases) // 独立实体
	v2 := make(map[Alias]EntityID, len(v1))
	for a, e := range v1 {
		v2[a] = e
	}
	v2["hot"] = 0 // 合并进实体 0，仅一条绑定不同

	if err := g.RegisterVersion("v1", v1); err != nil {
		t.Fatal(err)
	}
	if err := g.RegisterVersion("v2", v2); err != nil {
		t.Fatal(err)
	}
	if err := g.ActivateVersion("v1"); err != nil {
		t.Fatal(err)
	}

	hotEvery := cfg.docs / cfg.hotDocs
	for d := 0; d < cfg.docs; d++ {
		sentences := make([][]Alias, 0, 4)
		remaining := cfg.mentionsPer
		for remaining > 0 {
			n := 1 + rng.Intn(5)
			if n > remaining {
				n = remaining
			}
			mentions := make([]Alias, n)
			for i := range mentions {
				// 前 100 个别名保留为稀有别名，不进入随机池。
				mentions[i] = Alias(fmt.Sprintf("al%d", 100+rng.Intn(cfg.aliases-100)))
			}
			sentences = append(sentences, mentions)
			remaining -= n
		}
		if d%hotEvery == 0 && d/hotEvery < cfg.hotDocs {
			sentences[0] = append(sentences[0], "hot")
		}
		g.UpsertDocument(DocID(fmt.Sprintf("doc%d", d)), sentences)
	}
	// 少量文档使用稀有别名，形成低度实体。
	for d := 0; d < 5; d++ {
		g.UpsertDocument(DocID(fmt.Sprintf("rare%d", d)), [][]Alias{
			{Alias(fmt.Sprintf("al%d", d)), Alias(fmt.Sprintf("al%d", d+1)), "al100"},
			{Alias(fmt.Sprintf("al%d", d+2))},
		})
	}
	return g
}

func switchDurations(g *Graph, rounds int) []time.Duration {
	durs := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		from, to := "v1", "v2"
		if i%2 == 1 {
			from, to = to, from
		}
		_ = from
		start := time.Now()
		if err := g.ActivateVersion(VersionID(to)); err != nil {
			panic(err)
		}
		durs = append(durs, time.Since(start))
	}
	return durs
}

func percentile(durs []time.Duration, p float64) time.Duration {
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func median(durs []time.Duration) time.Duration { return percentile(durs, 0.5) }

// TestScaleVersionSwitch 验证：10万文档、200万提及、2万别名规模下，
// 仅一条绑定不同（涉及别名出现在不超过 20 篇文档）的版本切换 P95 不超过 100ms。
func TestScaleVersionSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in -short mode")
	}
	g := buildScaleGraph(t, scaleCorpus{docs: 100_000, aliases: 20_000, mentionsPer: 20, hotDocs: 20})
	switchDurations(g, 3) // 预热
	durs := switchDurations(g, 30)
	p95 := percentile(durs, 0.95)
	t.Logf("version switch at 100k docs: p95=%v median=%v", p95, median(durs))
	if p95 > 100*time.Millisecond {
		t.Fatalf("version switch p95 = %v, exceeds 100ms", p95)
	}
}

// TestScaleSwitchLocality 验证：语料从 1 万扩至 10 万（新增文档与切换涉及的
// 别名、实体完全无关）时，相同切换操作耗时中位数不超过原来的 3 倍。
func TestScaleSwitchLocality(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in -short mode")
	}
	small := buildScaleGraph(t, scaleCorpus{docs: 10_000, aliases: 20_000, mentionsPer: 20, hotDocs: 20})
	switchDurations(small, 3)
	med10k := median(switchDurations(small, 30))

	large := buildScaleGraph(t, scaleCorpus{docs: 100_000, aliases: 20_000, mentionsPer: 20, hotDocs: 20})
	switchDurations(large, 3)
	med100k := median(switchDurations(large, 30))

	t.Logf("switch median: 10k=%v 100k=%v ratio=%.2f", med10k, med100k, float64(med100k)/float64(med10k))
	if med100k > 3*med10k {
		t.Fatalf("switch median grew from %v to %v (>3x)", med10k, med100k)
	}
}

// TestScaleNeighborQuery 验证：具有不超过 100 条原始邻边的节点，
// 邻居查询 P95 不超过 30ms。
func TestScaleNeighborQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in -short mode")
	}
	g := buildScaleGraph(t, scaleCorpus{docs: 100_000, aliases: 20_000, mentionsPer: 20, hotDocs: 20})

	// 构造一个接近上限的节点：实体 50050（别名 al50）出现在 200 篇文档中，
	// 每篇搭配 al100..al199 中的一个，不同邻居不超过 100 个。
	rng := rand.New(rand.NewSource(11))
	for d := 0; d < 200; d++ {
		g.UpsertDocument(DocID(fmt.Sprintf("mid%d", d)), [][]Alias{
			{"al50", Alias(fmt.Sprintf("al%d", 100+rng.Intn(100)))},
			{Alias(fmt.Sprintf("al%d", 100+rng.Intn(100)))},
		})
	}

	// 找一个原始邻边（不同邻居）不超过 100 条但确实存在的实体。
	var target EntityID = 1 << 62
	for e := EntityID(50_050); e < 50_051; e++ {
		neighbors := make(map[EntityID]struct{})
		for docID := range g.entityDocs[e] {
			for p := range g.docs[docID].contrib.pairs {
				if p.A == e {
					neighbors[p.B] = struct{}{}
				} else if p.B == e {
					neighbors[p.A] = struct{}{}
				}
			}
		}
		if len(neighbors) > 0 && len(neighbors) <= 100 {
			target = e
			t.Logf("entity %d has %d raw edges", e, len(neighbors))
			break
		}
	}
	if target == 1<<62 {
		t.Fatal("no suitable entity found")
	}
	t.Logf("neighbor query target entity=%d docFreq=%d", target, g.DocumentFrequency(target))

	const rounds = 200
	durs := make([]time.Duration, 0, rounds)
	for i := 0; i < rounds; i++ {
		start := time.Now()
		g.Neighbors(target)
		durs = append(durs, time.Since(start))
	}
	p95 := percentile(durs, 0.95)
	t.Logf("neighbor query: p95=%v median=%v", p95, median(durs))
	if p95 > 30*time.Millisecond {
		t.Fatalf("neighbor query p95 = %v, exceeds 30ms", p95)
	}
}
