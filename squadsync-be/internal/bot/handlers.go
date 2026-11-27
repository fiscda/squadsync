package bot

// registerAll registers every command, component and modal handler of the
// bot. Add new ones here; keep each feature's handlers in its own file.
func (r *Router) registerAll() {
	r.Command(r.csatornaCommand())
}
