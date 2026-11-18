package web

import "testing"

func TestParseBoundedInt(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		// 缺省：调用方给的默认值（topology/impact 都是 1）
		{"empty falls back to default", "", 1, false},
		{"blank falls back to default", "   ", 1, false},
		{"valid lower bound", "1", 1, false},
		{"valid upper bound", "10", 10, false},
		{"valid middle", "3", 3, false},
		{"surrounding spaces tolerated", " 2 ", 2, false},
		// 以下都是「此前会被静默改成另一个语义」的输入
		{"zero rejected", "0", 1, true},
		{"negative rejected", "-3", 1, true},
		{"over max rejected", "11", 1, true},
		{"non numeric rejected", "abc", 1, true},
		{"float rejected", "2.5", 1, true},
		{"huge value rejected", "1000000", 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errMsg := parseBoundedInt(c.raw, "depth", 1, 1, maxGraphDepth)
			if c.wantErr {
				if errMsg == "" {
					t.Fatalf("parseBoundedInt(%q) errMsg = \"\", want error", c.raw)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("parseBoundedInt(%q) errMsg = %q, want no error", c.raw, errMsg)
			}
			if got != c.want {
				t.Fatalf("parseBoundedInt(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestParseNonNegativeInt64(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int64
		wantErr bool
	}{
		{"empty is zero", "", 0, false},
		{"blank is zero", "  ", 0, false},
		{"explicit zero allowed", "0", 0, false},
		{"positive value", "41", 41, false},
		{"surrounding spaces tolerated", " 5 ", 5, false},
		{"negative rejected", "-1", 0, true},
		{"non numeric rejected", "abc", 0, true},
		{"float rejected", "1.5", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errMsg := parseNonNegativeInt64(c.raw, "level")
			if c.wantErr {
				if errMsg == "" {
					t.Fatalf("parseNonNegativeInt64(%q) errMsg = \"\", want error", c.raw)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("parseNonNegativeInt64(%q) errMsg = %q, want no error", c.raw, errMsg)
			}
			if got != c.want {
				t.Fatalf("parseNonNegativeInt64(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestParseTopologyDirection(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"empty defaults to both", "", "both", false},
		{"blank defaults to both", "  ", "both", false},
		{"both", "both", "both", false},
		{"outgoing", "outgoing", "outgoing", false},
		{"incoming", "incoming", "incoming", false},
		{"surrounding spaces tolerated", " outgoing ", "outgoing", false},
		// 未知取值此前会让两个遍历分支都不进入，返回只有根节点的空图
		{"out rejected", "out", "both", true},
		{"uppercase rejected", "BOTH", "both", true},
		{"typo rejected", "incomming", "both", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errMsg := parseTopologyDirection(c.raw)
			if c.wantErr {
				if errMsg == "" {
					t.Fatalf("parseTopologyDirection(%q) errMsg = \"\", want error", c.raw)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("parseTopologyDirection(%q) errMsg = %q, want no error", c.raw, errMsg)
			}
			if got != c.want {
				t.Fatalf("parseTopologyDirection(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

// TestParsePositiveInt64 覆盖关系列表筛选参数：DAO 用 `> 0` 判断是否加过滤条件
// （internal/repository/dao/relation.go:186/189），所以 0 / 负数 / 非数字都会让
// 筛选条件整体消失、返回全库关系，必须在此拒绝。
func TestParsePositiveInt64(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int64
		wantErr bool
	}{
		{"empty means no filter", "", 0, false},
		{"blank means no filter", "   ", 0, false},
		{"positive", "1367", 1367, false},
		{"surrounding spaces tolerated", " 383450 ", 383450, false},
		{"large id", "9007199254740993", 9007199254740993, false},
		{"zero rejected", "0", 0, true},
		{"negative rejected", "-5", 0, true},
		{"non numeric rejected", "abc", 0, true},
		{"sql-ish string rejected", "1 OR 1=1", 0, true},
		{"float rejected", "1.5", 0, true},
		{"overflow rejected", "99999999999999999999", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errMsg := parsePositiveInt64(c.raw, "source_instance_id")
			if c.wantErr {
				if errMsg == "" {
					t.Fatalf("parsePositiveInt64(%q) errMsg = \"\", want error", c.raw)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("parsePositiveInt64(%q) errMsg = %q, want no error", c.raw, errMsg)
			}
			if got != c.want {
				t.Fatalf("parsePositiveInt64(%q) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

// TestMaxGraphDepthCoversFrontendUsage 守卫上限常量：前端影响分析允许选到 10 层
// （views/impact/index.vue 的 el-input-number :max="10"），拓扑页到 3 层。
// 上限一旦被调低，这两个页面的合法请求会被新校验拒掉。
func TestMaxGraphDepthCoversFrontendUsage(t *testing.T) {
	if maxGraphDepth < 10 {
		t.Fatalf("maxGraphDepth = %d, want >= 10 (impact 页面允许选到 10 层)", maxGraphDepth)
	}
	if _, errMsg := parseBoundedInt("10", "depth", 1, 1, maxGraphDepth); errMsg != "" {
		t.Fatalf("depth=10 应被接受（影响分析前端上限），得到错误 %q", errMsg)
	}
}
