package param

func DetectCategory(code string) StockCategory {
	// Simple heuristic: first 3 digits of code
	// A-shares: 600xxx/601xxx/603xxx = Shanghai large/mid
	//           000xxx/001xxx = Shenzhen large/mid
	//           002xxx = Shenzhen small cap
	//           300xxx = ChiNext (mid/small)
	//           688xxx = STAR Market (mid/small)
	if len(code) < 3 {
		return CategoryMidCap
	}

	prefix := code[:3]
	switch prefix {
	case "600", "601", "603", "605":
		return CategoryLargeCap
	case "000", "001":
		return CategoryLargeCap
	case "002":
		return CategorySmallCap
	case "300":
		return CategoryMidCap
	case "688":
		return CategoryMidCap
	default:
		return CategoryMidCap
	}
}

func ParseCategory(s string) StockCategory {
	switch s {
	case "large_cap", "大盘股":
		return CategoryLargeCap
	case "mid_cap", "中盘股":
		return CategoryMidCap
	case "small_cap", "小盘股":
		return CategorySmallCap
	default:
		return CategoryMidCap
	}
}
