package runtime

import (
	"strings"
	"testing"
)

func TestRaylibBuiltinRequiresInitializedWindow(t *testing.T) {
	runtime := New()
	_, err := runtime.callRaylibBuiltin("raylib_begin_drawing", nil)
	if err == nil || !strings.Contains(err.Error(), "requires InitWindow") {
		t.Fatalf("expected an uninitialized-window error, got %v", err)
	}
}

func TestRaylibBuiltinRejectsWorkerRuntime(t *testing.T) {
	runtime := New()
	runtime.worker = true
	_, err := runtime.callRaylibBuiltin("raylib_is_window_ready", nil)
	if err == nil || !strings.Contains(err.Error(), "main kLang runtime") {
		t.Fatalf("expected a main-runtime error, got %v", err)
	}
}

func TestRaylibColorValidatesComponents(t *testing.T) {
	_, err := raylibColor("raylib_clear_background", []Value{
		IntValue(256), IntValue(0), IntValue(0), IntValue(255),
	})
	if err == nil || !strings.Contains(err.Error(), "between 0 and 255") {
		t.Fatalf("expected a color range error, got %v", err)
	}
}

func TestRaylibCollisionBuiltinsRunWithoutWindow(t *testing.T) {
	runtime := New()
	overlap, err := runtime.callRaylibBuiltin("raylib_check_collision_recs", []Value{
		IntValue(0), IntValue(0), IntValue(10), IntValue(10),
		IntValue(5), IntValue(5), IntValue(10), IntValue(10),
	})
	if err != nil {
		t.Fatalf("rectangle collision failed: %v", err)
	}
	if overlap.Kind != ValueBool || !overlap.Data.(bool) {
		t.Fatalf("expected overlapping rectangles, got %#v", overlap)
	}

	inside, err := runtime.callRaylibBuiltin("raylib_check_collision_point_rec", []Value{
		IntValue(4), IntValue(4), IntValue(0), IntValue(0), IntValue(10), IntValue(10),
	})
	if err != nil {
		t.Fatalf("point collision failed: %v", err)
	}
	if inside.Kind != ValueBool || !inside.Data.(bool) {
		t.Fatalf("expected point inside rectangle, got %#v", inside)
	}
}

func TestRaylibRandomValueValidatesBounds(t *testing.T) {
	runtime := New()
	_, err := runtime.callRaylibBuiltin("raylib_get_random_value", []Value{IntValue(10), IntValue(1)})
	if err == nil || !strings.Contains(err.Error(), "minimum cannot exceed maximum") {
		t.Fatalf("expected invalid random bounds error, got %v", err)
	}
}
