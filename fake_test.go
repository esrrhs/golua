package fakescript_go

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTempScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.fk")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp script: %v", err)
	}
	return filePath
}

func TestDefaultConfigInit(t *testing.T) {
	script := `
package testpkg

func add(a, b)
	return a + b
end
`
	file := createTempScript(t, script)
	err := Parse(file)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ret, err := Run("testpkg.add", 10, 25)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 35 {
		t.Fatalf("expected [35], got %v", ret)
	}
}

func TestSetConfigAndPrint(t *testing.T) {
	var printedOutput string
	SetConfig(FakeConfig{
		OpenLog:        false,
		ArrayGrowSpeed: 20,
		StackMax:       5000,
		FakePrint: func(str string) {
			printedOutput = str
		},
	})

	script := `
package testprint

func do_print(msg)
	print(msg)
	return 1
end
`
	file := createTempScript(t, script)
	err := Parse(file)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ret, err := Run("testprint.do_print", "hello fakescript_go")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 1 {
		t.Fatalf("unexpected return: %v", ret)
	}
	if !strings.Contains(printedOutput, "hello fakescript_go") {
		t.Fatalf("expected printedOutput to contain 'hello fakescript_go', got %q", printedOutput)
	}
}

func TestRegFuncAndCall(t *testing.T) {
	err := RegFunc("multiply_go", func(a int, b int) int {
		return a * b
	})
	if err != nil {
		t.Fatalf("RegFunc failed: %v", err)
	}

	script := `
package testbind

func calc(x, y)
	return multiply_go(x, y)
end
`
	file := createTempScript(t, script)
	if err := Parse(file); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ret, err := Run("testbind.calc", 6, 7)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 42 {
		t.Fatalf("expected 42, got %v", ret)
	}
}

func TestParseSyntaxError(t *testing.T) {
	badScript := `
package bad
func broken(
`
	file := createTempScript(t, badScript)
	err := Parse(file)
	if err == nil {
		t.Fatalf("expected syntax error, got nil")
	}
}

func TestRunNonExistentFunc(t *testing.T) {
	_, err := Run("nonexistent.func_name", 1)
	if err == nil {
		t.Fatalf("expected error for non-existent function, got nil")
	}
}

func TestDebugRun(t *testing.T) {
	script := `
package testdebug

func get_single(x)
	return x * 2
end
`
	file := createTempScript(t, script)
	if err := Parse(file); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// DebugRun is currently a stub in fakego, ensure it does not panic and returns without error
	_, err := DebugRun("testdebug.get_single", 21)
	if err != nil {
		t.Fatalf("DebugRun failed: %v", err)
	}
}

func TestParamstackPops(t *testing.T) {
	ps := &paramstack{}
	ps.push(1)
	ps.push(2)
	ps.push(3)
	if ps.size() != 3 {
		t.Fatalf("expected size 3, got %d", ps.size())
	}
	items := ps.pops()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Items popped in LIFO order
	if items[0] != 3 || items[1] != 2 || items[2] != 1 {
		t.Fatalf("unexpected pop order: %v", items)
	}
	if ps.size() != 0 {
		t.Fatalf("expected empty stack after pops, got size %d", ps.size())
	}
}

func TestRoutineAndSleepYield(t *testing.T) {
	var results []int
	err := RegFunc("record_val", func(v int) {
		results = append(results, v)
	})
	if err != nil {
		t.Fatalf("RegFunc failed: %v", err)
	}

	script := `
package testroutine

func task(id)
	yield 1
	record_val(id)
	sleep 10
	record_val(id * 10)
end

func main_task()
	fake task(1)
	fake task(2)
	record_val(0)
	return 999
end
`
	file := createTempScript(t, script)
	if err := Parse(file); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ret, err := Run("testroutine.main_task")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 999 {
		t.Fatalf("expected 999, got %v", ret)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d: %v", len(results), results)
	}
}

