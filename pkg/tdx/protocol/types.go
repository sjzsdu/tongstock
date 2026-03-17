package protocol

const (
	KindIndex = "index"
	KindStock = "stock"
)

const (
	TypeKline5Minute  uint8 = 0
	TypeKline15Minute uint8 = 1
	TypeKline30Minute uint8 = 2
	TypeKline60Minute uint8 = 3
	TypeKlineHour     uint8 = 3
	TypeKlineDay2     uint8 = 4
	TypeKlineWeek     uint8 = 5
	TypeKlineMonth    uint8 = 6
	TypeKlineMinute   uint8 = 7
	TypeKlineMinute2  uint8 = 8
	TypeKlineDay      uint8 = 9
	TypeKlineQuarter  uint8 = 10
	TypeKlineYear     uint8 = 11
)
