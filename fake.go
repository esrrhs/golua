package fakego

var gfs fakeStruct

type fakeStruct struct {
	cfg FakeConfig
	pa  parser
	con container
}
