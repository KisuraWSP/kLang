//go:build !js

package runtime

import (
	"fmt"
	"math"
	stdruntime "runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func (runtime *Runtime) callRaylibBuiltin(name string, args []Value) (Value, error) {
	if runtime.worker {
		return NullValue(), Error{Message: name + " must be called from the main kLang runtime"}
	}

	switch name {
	case "raylib_init_window":
		if err := expectRaylibArgs(name, args, ValueInt, ValueInt, ValueString); err != nil {
			return NullValue(), err
		}
		if runtime.raylibWindowOpen {
			return NullValue(), Error{Message: "raylib window is already initialized"}
		}
		width, err := raylibInt32(name, "width", args[0], true)
		if err != nil {
			return NullValue(), err
		}
		height, err := raylibInt32(name, "height", args[1], true)
		if err != nil {
			return NullValue(), err
		}
		stdruntime.LockOSThread()
		runtime.raylibThreadLocked = true
		rl.InitWindow(width, height, args[2].Data.(string))
		if !rl.IsWindowReady() {
			runtime.closeRaylib()
			return NullValue(), Error{Message: "raylib failed to initialize the native window"}
		}
		runtime.raylibWindowOpen = true
		return NullValue(), nil
	case "raylib_close_window":
		if err := expectRaylibArgs(name, args); err != nil {
			return NullValue(), err
		}
		runtime.closeRaylib()
		return NullValue(), nil
	case "raylib_window_should_close":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return BoolValue(rl.WindowShouldClose()), nil
	case "raylib_is_window_ready":
		if err := expectRaylibArgs(name, args); err != nil {
			return NullValue(), err
		}
		return BoolValue(runtime.raylibWindowOpen && rl.IsWindowReady()), nil
	case "raylib_set_target_fps":
		if err := runtime.expectRaylibWindow(name, args, ValueInt); err != nil {
			return NullValue(), err
		}
		fps, err := raylibInt32(name, "fps", args[0], true)
		if err != nil {
			return NullValue(), err
		}
		rl.SetTargetFPS(fps)
		return NullValue(), nil
	case "raylib_get_fps":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return IntValue(int(rl.GetFPS())), nil
	case "raylib_get_frame_time":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return FloatValue(float64(rl.GetFrameTime())), nil
	case "raylib_begin_drawing":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		rl.BeginDrawing()
		return NullValue(), nil
	case "raylib_end_drawing":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		rl.EndDrawing()
		return NullValue(), nil
	case "raylib_clear_background":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args)
		if err != nil {
			return NullValue(), err
		}
		rl.ClearBackground(color)
		return NullValue(), nil
	case "raylib_draw_text":
		if err := runtime.expectRaylibWindow(name, args, ValueString, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		x, err := raylibInt32(name, "x", args[1], false)
		if err != nil {
			return NullValue(), err
		}
		y, err := raylibInt32(name, "y", args[2], false)
		if err != nil {
			return NullValue(), err
		}
		size, err := raylibInt32(name, "font size", args[3], true)
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[4:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawText(args[0].Data.(string), x, y, size, color)
		return NullValue(), nil
	case "raylib_draw_rectangle":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"x", "y", "width", "height"}, args[:4], []bool{false, false, true, true})
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[4:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawRectangle(values[0], values[1], values[2], values[3], color)
		return NullValue(), nil
	case "raylib_draw_circle":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"x", "y", "radius"}, args[:3], []bool{false, false, true})
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[3:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawCircle(values[0], values[1], float32(values[2]), color)
		return NullValue(), nil
	case "raylib_is_key_pressed", "raylib_is_key_down":
		if err := runtime.expectRaylibWindow(name, args, ValueInt); err != nil {
			return NullValue(), err
		}
		key, err := raylibInt32(name, "key", args[0], false)
		if err != nil {
			return NullValue(), err
		}
		if name == "raylib_is_key_pressed" {
			return BoolValue(rl.IsKeyPressed(key)), nil
		}
		return BoolValue(rl.IsKeyDown(key)), nil
	case "raylib_get_screen_width":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return IntValue(int(rl.GetScreenWidth())), nil
	case "raylib_get_screen_height":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return IntValue(int(rl.GetScreenHeight())), nil
	case "raylib_set_window_title":
		if err := runtime.expectRaylibWindow(name, args, ValueString); err != nil {
			return NullValue(), err
		}
		rl.SetWindowTitle(args[0].Data.(string))
		return NullValue(), nil
	case "raylib_set_window_size", "raylib_set_window_position", "raylib_set_mouse_position":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		positive := []bool{false, false}
		labels := []string{"x", "y"}
		if name == "raylib_set_window_size" {
			positive = []bool{true, true}
			labels = []string{"width", "height"}
		}
		values, err := raylibInt32Arguments(name, labels, args, positive)
		if err != nil {
			return NullValue(), err
		}
		switch name {
		case "raylib_set_window_size":
			rl.SetWindowSize(int(values[0]), int(values[1]))
		case "raylib_set_window_position":
			rl.SetWindowPosition(int(values[0]), int(values[1]))
		default:
			rl.SetMousePosition(int(values[0]), int(values[1]))
		}
		return NullValue(), nil
	case "raylib_toggle_fullscreen", "raylib_maximize_window", "raylib_minimize_window", "raylib_restore_window":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		switch name {
		case "raylib_toggle_fullscreen":
			rl.ToggleFullscreen()
		case "raylib_maximize_window":
			rl.MaximizeWindow()
		case "raylib_minimize_window":
			rl.MinimizeWindow()
		default:
			rl.RestoreWindow()
		}
		return NullValue(), nil
	case "raylib_is_window_fullscreen", "raylib_is_window_hidden", "raylib_is_window_minimized",
		"raylib_is_window_maximized", "raylib_is_window_focused":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		switch name {
		case "raylib_is_window_fullscreen":
			return BoolValue(rl.IsWindowFullscreen()), nil
		case "raylib_is_window_hidden":
			return BoolValue(rl.IsWindowHidden()), nil
		case "raylib_is_window_minimized":
			return BoolValue(rl.IsWindowMinimized()), nil
		case "raylib_is_window_maximized":
			return BoolValue(rl.IsWindowMaximized()), nil
		default:
			return BoolValue(rl.IsWindowFocused()), nil
		}
	case "raylib_get_time":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return FloatValue(rl.GetTime()), nil
	case "raylib_set_exit_key":
		if err := runtime.expectRaylibWindow(name, args, ValueInt); err != nil {
			return NullValue(), err
		}
		key, err := raylibInt32(name, "key", args[0], false)
		if err != nil {
			return NullValue(), err
		}
		rl.SetExitKey(key)
		return NullValue(), nil
	case "raylib_is_key_pressed_repeat", "raylib_is_key_released", "raylib_is_key_up":
		if err := runtime.expectRaylibWindow(name, args, ValueInt); err != nil {
			return NullValue(), err
		}
		key, err := raylibInt32(name, "key", args[0], false)
		if err != nil {
			return NullValue(), err
		}
		switch name {
		case "raylib_is_key_pressed_repeat":
			return BoolValue(rl.IsKeyPressedRepeat(key)), nil
		case "raylib_is_key_released":
			return BoolValue(rl.IsKeyReleased(key)), nil
		default:
			return BoolValue(rl.IsKeyUp(key)), nil
		}
	case "raylib_get_key_pressed", "raylib_get_char_pressed":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		if name == "raylib_get_key_pressed" {
			return IntValue(int(rl.GetKeyPressed())), nil
		}
		return IntValue(int(rl.GetCharPressed())), nil
	case "raylib_is_mouse_button_pressed", "raylib_is_mouse_button_down", "raylib_is_mouse_button_released":
		if err := runtime.expectRaylibWindow(name, args, ValueInt); err != nil {
			return NullValue(), err
		}
		button, err := raylibInt32(name, "button", args[0], false)
		if err != nil {
			return NullValue(), err
		}
		switch name {
		case "raylib_is_mouse_button_pressed":
			return BoolValue(rl.IsMouseButtonPressed(rl.MouseButton(button))), nil
		case "raylib_is_mouse_button_down":
			return BoolValue(rl.IsMouseButtonDown(rl.MouseButton(button))), nil
		default:
			return BoolValue(rl.IsMouseButtonReleased(rl.MouseButton(button))), nil
		}
	case "raylib_get_mouse_x", "raylib_get_mouse_y":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		if name == "raylib_get_mouse_x" {
			return IntValue(int(rl.GetMouseX())), nil
		}
		return IntValue(int(rl.GetMouseY())), nil
	case "raylib_get_mouse_wheel_move":
		if err := runtime.expectRaylibWindow(name, args); err != nil {
			return NullValue(), err
		}
		return FloatValue(float64(rl.GetMouseWheelMove())), nil
	case "raylib_draw_pixel":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"x", "y"}, args[:2], []bool{false, false})
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[2:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawPixel(values[0], values[1], color)
		return NullValue(), nil
	case "raylib_draw_line", "raylib_draw_rectangle_lines":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		labels := []string{"start x", "start y", "end x", "end y"}
		positive := []bool{false, false, false, false}
		if name == "raylib_draw_rectangle_lines" {
			labels = []string{"x", "y", "width", "height"}
			positive = []bool{false, false, true, true}
		}
		values, err := raylibInt32Arguments(name, labels, args[:4], positive)
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[4:])
		if err != nil {
			return NullValue(), err
		}
		if name == "raylib_draw_line" {
			rl.DrawLine(values[0], values[1], values[2], values[3], color)
		} else {
			rl.DrawRectangleLines(values[0], values[1], values[2], values[3], color)
		}
		return NullValue(), nil
	case "raylib_draw_circle_lines":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"x", "y", "radius"}, args[:3], []bool{false, false, true})
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[3:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawCircleLines(values[0], values[1], float32(values[2]), color)
		return NullValue(), nil
	case "raylib_draw_ellipse":
		if err := runtime.expectRaylibWindow(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"x", "y", "horizontal radius", "vertical radius"}, args[:4], []bool{false, false, true, true})
		if err != nil {
			return NullValue(), err
		}
		color, err := raylibColor(name, args[4:])
		if err != nil {
			return NullValue(), err
		}
		rl.DrawEllipse(values[0], values[1], float32(values[2]), float32(values[3]), color)
		return NullValue(), nil
	case "raylib_measure_text":
		if err := runtime.expectRaylibWindow(name, args, ValueString, ValueInt); err != nil {
			return NullValue(), err
		}
		size, err := raylibInt32(name, "font size", args[1], true)
		if err != nil {
			return NullValue(), err
		}
		return IntValue(int(rl.MeasureText(args[0].Data.(string), size))), nil
	case "raylib_take_screenshot":
		if err := runtime.expectRaylibWindow(name, args, ValueString); err != nil {
			return NullValue(), err
		}
		rl.TakeScreenshot(args[0].Data.(string))
		return NullValue(), nil
	case "raylib_get_random_value":
		if err := expectRaylibArgs(name, args, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name, []string{"minimum", "maximum"}, args, []bool{false, false})
		if err != nil {
			return NullValue(), err
		}
		if values[0] > values[1] {
			return NullValue(), Error{Message: "raylib_get_random_value minimum cannot exceed maximum"}
		}
		return IntValue(int(rl.GetRandomValue(values[0], values[1]))), nil
	case "raylib_check_collision_recs":
		if err := expectRaylibArgs(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name,
			[]string{"left x", "left y", "left width", "left height", "right x", "right y", "right width", "right height"},
			args, []bool{false, false, true, true, false, false, true, true})
		if err != nil {
			return NullValue(), err
		}
		left := rl.Rectangle{X: float32(values[0]), Y: float32(values[1]), Width: float32(values[2]), Height: float32(values[3])}
		right := rl.Rectangle{X: float32(values[4]), Y: float32(values[5]), Width: float32(values[6]), Height: float32(values[7])}
		return BoolValue(rl.CheckCollisionRecs(left, right)), nil
	case "raylib_check_collision_circles":
		if err := expectRaylibArgs(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name,
			[]string{"first x", "first y", "first radius", "second x", "second y", "second radius"},
			args, []bool{false, false, true, false, false, true})
		if err != nil {
			return NullValue(), err
		}
		first := rl.Vector2{X: float32(values[0]), Y: float32(values[1])}
		second := rl.Vector2{X: float32(values[3]), Y: float32(values[4])}
		return BoolValue(rl.CheckCollisionCircles(first, float32(values[2]), second, float32(values[5]))), nil
	case "raylib_check_collision_point_rec":
		if err := expectRaylibArgs(name, args, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt, ValueInt); err != nil {
			return NullValue(), err
		}
		values, err := raylibInt32Arguments(name,
			[]string{"point x", "point y", "rectangle x", "rectangle y", "rectangle width", "rectangle height"},
			args, []bool{false, false, false, false, true, true})
		if err != nil {
			return NullValue(), err
		}
		point := rl.Vector2{X: float32(values[0]), Y: float32(values[1])}
		rectangle := rl.Rectangle{X: float32(values[2]), Y: float32(values[3]), Width: float32(values[4]), Height: float32(values[5])}
		return BoolValue(rl.CheckCollisionPointRec(point, rectangle)), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unknown raylib builtin %q", name)}
	}
}

func (runtime *Runtime) expectRaylibWindow(name string, args []Value, kinds ...ValueKind) error {
	if err := expectRaylibArgs(name, args, kinds...); err != nil {
		return err
	}
	if !runtime.raylibWindowOpen {
		return Error{Message: name + " requires InitWindow to be called first"}
	}
	return nil
}

func expectRaylibArgs(name string, args []Value, kinds ...ValueKind) error {
	if len(args) != len(kinds) {
		return Error{Message: fmt.Sprintf("%s expects %d argument(s), got %d", name, len(kinds), len(args))}
	}
	for index, kind := range kinds {
		if args[index].Kind != kind {
			return Error{Message: fmt.Sprintf("%s argument %d expects %s, got %s", name, index+1, kind, args[index].Kind)}
		}
	}
	return nil
}

func raylibInt32(name string, label string, value Value, positive bool) (int32, error) {
	number := value.Data.(int)
	if number < math.MinInt32 || number > math.MaxInt32 {
		return 0, Error{Message: fmt.Sprintf("%s %s is outside the supported 32-bit range", name, label)}
	}
	if positive && number <= 0 {
		return 0, Error{Message: fmt.Sprintf("%s %s must be greater than zero", name, label)}
	}
	return int32(number), nil
}

func raylibInt32Arguments(name string, labels []string, args []Value, positive []bool) ([]int32, error) {
	values := make([]int32, len(args))
	for index, arg := range args {
		value, err := raylibInt32(name, labels[index], arg, positive[index])
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

func raylibColor(name string, args []Value) (rl.Color, error) {
	components := [4]uint8{}
	labels := []string{"red", "green", "blue", "alpha"}
	for index, arg := range args {
		value := arg.Data.(int)
		if value < 0 || value > 255 {
			return rl.Color{}, Error{Message: fmt.Sprintf("%s color %s must be between 0 and 255", name, labels[index])}
		}
		components[index] = uint8(value)
	}
	return rl.NewColor(components[0], components[1], components[2], components[3]), nil
}

func (runtime *Runtime) closeRaylib() {
	if runtime.raylibWindowOpen {
		rl.CloseWindow()
		runtime.raylibWindowOpen = false
	}
	if runtime.raylibThreadLocked {
		runtime.raylibThreadLocked = false
		stdruntime.UnlockOSThread()
	}
}
