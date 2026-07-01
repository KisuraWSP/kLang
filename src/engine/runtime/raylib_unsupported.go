//go:build js

package runtime

func (runtime *Runtime) callRaylibBuiltin(name string, args []Value) (Value, error) {
	return NullValue(), Error{Message: name + " is unavailable in the browser runtime"}
}

func (runtime *Runtime) closeRaylib() {
	runtime.raylibWindowOpen = false
	runtime.raylibThreadLocked = false
}
