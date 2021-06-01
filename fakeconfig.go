package fakego

type FakeConfig struct {
	OpenLog        bool          // 开启日志
	ArrayGrowSpeed int           // 增长速度，百分比，10%代表增长10%
	StackMax       int           // stack最大尺寸
	OpenProfile    bool          // 开启性能统计
	FakePrint      FakePrintFunc // 打印函数
}

type FakePrintFunc func(str string)

func (fc *FakeConfig) check() {
	if fc.ArrayGrowSpeed <= 0 {
		fc.ArrayGrowSpeed = 50
	}
	if fc.StackMax <= 0 {
		fc.StackMax = 10000
	}
}

func SetConfig(fc FakeConfig) {
	fc.check()
	gfs.cfg = fc
}
