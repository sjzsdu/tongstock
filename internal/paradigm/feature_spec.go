package paradigm

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// FeatureCategory 特征分类, 用于组织和筛选。
type FeatureCategory string

const (
	FeatureCategoryTechnical        FeatureCategory = "technical"         // TA 技术指标 (MA, MACD, KDJ, BOLL, RSI)
	FeatureCategoryVolumePrice      FeatureCategory = "volume_price"      // 量价关系 (成交量比率, 量价背离, OBV)
	FeatureCategoryRelativeStrength FeatureCategory = "relative_strength" // 相对强弱 (RPS, 相对指数)
	FeatureCategoryMarketState      FeatureCategory = "market_state"      // 市场状态 (趋势, 震荡, 动量)
	FeatureCategoryFinancial        FeatureCategory = "financial"         // 财务指标 (PE, PB, ROE, 增长率)
	FeatureCategoryEvent            FeatureCategory = "event"             // 事件特征 (分红, 拆股, 停牌, ST)
)

// ComputationTiming 计算时点类型, 用于无泄漏校验。
type ComputationTiming string

const (
	TimingEndOfDay    ComputationTiming = "end_of_day"   // 日终数据, T+1 开盘后可用
	TimingIntraday    ComputationTiming = "intraday"     // 盘中数据, 实时可用
	TimingNextDay     ComputationTiming = "next_day"     // 次日数据 (财务, 公告)
	TimingQuarterly   ComputationTiming = "quarterly"    // 季度数据 (财报)
	TimingEventDriven ComputationTiming = "event_driven" // 事件驱动 (分红除权等)
)

// FeatureSpec 统一特征规格定义, 覆盖 TA 指标、量价、相对强弱、市场状态、财务和事件特征。
// 每个 FeatureSpec 带版本、窗口、依赖和可获得时间, 确保 CLI/HTTP/回测/AI 使用同一特征定义。
type FeatureSpec struct {
	ID          string          `json:"id"`       // e.g. "MACD", "RSI", "VolumeRatio"
	Name        string          `json:"name"`     // 可读名称
	Category    FeatureCategory `json:"category"` // 特征分类
	Version     int             `json:"version"`  // 版本号, 公式变更时递增
	Description string          `json:"description"`

	// 参数与窗口
	DefaultParams map[string]interface{} `json:"default_params"` // 默认参数, e.g. {"fast":12,"slow":26,"signal":9}
	Window        int                    `json:"window"`         // 回看窗口 (K线根数), 0 = 自适应
	MinSamples    int                    `json:"min_samples"`    // 最少需要的样本数

	// 依赖关系
	Dependencies []string `json:"dependencies,omitempty"` // 依赖的其他特征 ID, 用于拓扑排序

	// 计算元数据
	Timing       ComputationTiming `json:"timing"`
	DataRequired []string          `json:"data_required"` // 需要的数据类型: "kline", "quote", "finance", "xdxr"

	// 公式指纹: 用于检测公式变更, 变更时自动产生新版本
	FormulaHash string `json:"formula_hash"`

	// 状态
	Status    string    `json:"status"` // active / deprecated
	CreatedAt time.Time `json:"created_at"`
}

// ComputeKey 返回特征的唯一标识: ID + Version。
func (f *FeatureSpec) ComputeKey() string {
	if f.Version > 0 {
		return fmt.Sprintf("%s@v%d", f.ID, f.Version)
	}
	return f.ID
}

// ParamInt 获取整型参数, 带默认值。
func (f *FeatureSpec) ParamInt(key string, defaultVal int) int {
	if v, ok := f.DefaultParams[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case json.Number:
			n, _ := val.Int64()
			return int(n)
		}
	}
	return defaultVal
}

// ParamFloat 获取浮点参数, 带默认值。
func (f *FeatureSpec) ParamFloat(key string, defaultVal float64) float64 {
	if v, ok := f.DefaultParams[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case json.Number:
			n, _ := val.Float64()
			return n
		}
	}
	return defaultVal
}

// ParamString 获取字符串参数。
func (f *FeatureSpec) ParamString(key, defaultVal string) string {
	if v, ok := f.DefaultParams[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// Validate 校验特征规格完整性。
func (f *FeatureSpec) Validate() error {
	if f.ID == "" {
		return fmt.Errorf("feature ID is required")
	}
	if f.Category == "" {
		return fmt.Errorf("feature category is required for %s", f.ID)
	}
	if f.Window < 0 {
		return fmt.Errorf("window must be non-negative for %s", f.ID)
	}
	return nil
}

// FeatureValue 单个特征在某一时刻的计算结果。
type FeatureValue struct {
	FeatureID   string    `json:"feature_id"`
	Version     int       `json:"version"`
	StockCode   string    `json:"stock_code"`
	Date        time.Time `json:"date"`
	Value       float64   `json:"value"`
	SourceData  string    `json:"source_data,omitempty"` // 数据版本信息
	ComputedAt  time.Time `json:"computed_at"`
	AsOf        time.Time `json:"as_of"`
	LeakChecked bool      `json:"leak_checked"`
}

// FeatureResult 一组特征的计算结果。
type FeatureResult struct {
	FeatureSetID string         `json:"feature_set_id"`
	StockCode    string         `json:"stock_code"`
	Values       []FeatureValue `json:"values"`
	ComputedAt   time.Time      `json:"computed_at"`
	AsOf         time.Time      `json:"as_of"` // 数据可获得时间
	LeakChecked  bool           `json:"leak_checked"`
}

// FeatureSetSpec 特征集合规格: 一组相关特征的打包定义。
type FeatureSetSpec struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     int             `json:"version"`
	Description string          `json:"description"`
	Features    []string        `json:"features"` // FeatureSpec ID 列表
	Category    FeatureCategory `json:"category"`
	PriceReq    PriceAdjustment `json:"price_req"` // 需要的价格口径
	CreatedAt   time.Time       `json:"created_at"`
}

// NewFeatureSetSpec 创建特征集合。
func NewFeatureSetSpec(id, name string, category FeatureCategory, features []string, priceReq PriceAdjustment) *FeatureSetSpec {
	return &FeatureSetSpec{
		ID:        id,
		Name:      name,
		Category:  category,
		Features:  features,
		PriceReq:  priceReq,
		CreatedAt: time.Now(),
	}
}

// HashFormula 计算特征规格的公式指纹。
// 用于检测公式/参数是否发生变更, 变更时 Registry 会自动产生新版本。
func HashFormula(formula string, params map[string]interface{}) string {
	data := formula
	if len(params) > 0 {
		b, _ := json.Marshal(params)
		data = formula + string(b)
	}
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

// LeakCheckResult 无泄漏检查结果。
type LeakCheckResult struct {
	Passed     bool      `json:"passed"`
	Violations []string  `json:"violations,omitempty"`
	FeatureID  string    `json:"feature_id"`
	CheckDate  time.Time `json:"check_date"`
	DataAsOf   time.Time `json:"data_as_of"`
}

// NewLeakCheck 创建无泄漏检查。
func NewLeakCheck(featureID string, checkDate, dataAsOf time.Time, timing ComputationTiming) *LeakCheckResult {
	result := &LeakCheckResult{
		FeatureID: featureID,
		CheckDate: checkDate,
		DataAsOf:  dataAsOf,
	}

	// 检查: 数据可获得时间不应晚于检查日期 + 延迟
	now := time.Now()
	var effectiveDeadline time.Time

	switch timing {
	case TimingEndOfDay:
		// T 日数据在 T+1 日开盘后可用
		effectiveDeadline = checkDate.AddDate(0, 0, 1)
	case TimingIntraday:
		effectiveDeadline = checkDate
	case TimingNextDay:
		effectiveDeadline = checkDate.AddDate(0, 0, 1)
	case TimingQuarterly:
		// 季度数据: 季度结束后约 15 天内披露
		effectiveDeadline = checkDate
	case TimingEventDriven:
		effectiveDeadline = checkDate
	default:
		effectiveDeadline = checkDate
	}

	if dataAsOf.After(effectiveDeadline) {
		result.Violations = append(result.Violations,
			fmt.Sprintf("data as_of (%s) is after effective deadline (%s), potential future data leak",
				dataAsOf.Format("2006-01-02"), effectiveDeadline.Format("2006-01-02")))
	}

	if checkDate.After(now) {
		result.Violations = append(result.Violations,
			fmt.Sprintf("check date (%s) is in the future", checkDate.Format("2006-01-02")))
	}

	result.Passed = len(result.Violations) == 0
	return result
}
